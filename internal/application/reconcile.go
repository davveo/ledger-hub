package application

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

type ReconcileService struct {
	entries domain.EntryRepository
	accs    domain.AccountRepository
	freezes domain.FreezeRepository
	store   domain.ReconcileRepository
}

func NewReconcileService(entries domain.EntryRepository, accs domain.AccountRepository, freezes domain.FreezeRepository, store domain.ReconcileRepository) *ReconcileService {
	return &ReconcileService{entries: entries, accs: accs, freezes: freezes, store: store}
}

type ReconcileReport struct {
	Job           *domain.ReconcileJob    `json:"job"`
	LedgerEntries []*domain.LedgerEntry   `json:"ledger_entries"`
	Diffs         []*domain.ReconcileDiff `json:"diffs"`
	Files         map[string]string       `json:"files"`
}

func (s *ReconcileService) Trigger(ctx context.Context, tenantID, date, sourceSystem, assetCode string, bizLines []domain.BizLine) (*ReconcileReport, error) {
	if tenantID == "" || date == "" {
		return nil, domain.ErrInvalidParam
	}
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, domain.NewError(domain.CodeInvalidParam, "date 需为 YYYY-MM-DD")
	}
	from := day.UTC()
	to := from.Add(24 * time.Hour)

	job := &domain.ReconcileJob{
		JobID:        idgen.New("rj_"),
		TenantID:     tenantID,
		Date:         date,
		SourceSystem: sourceSystem,
		AssetCode:    assetCode,
		Status:       domain.ReconJobRunning,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.store.CreateJob(ctx, job); err != nil {
		return nil, err
	}

	entries, err := s.entries.ListByRange(ctx, tenantID, sourceSystem, assetCode, from, to)
	if err != nil {
		job.Status = domain.ReconJobFailed
		job.Note = err.Error()
		_ = s.store.UpdateJob(ctx, job)
		return nil, err
	}

	var inAmt, outAmt int64
	for _, e := range entries {
		if e.Direction == domain.DirIN {
			inAmt += e.Amount
		} else {
			outAmt += e.Amount
		}
	}

	diffs := MatchBizLines(job.JobID, bizLines, entries)
	l2, err := s.balanceTieOut(ctx, tenantID, assetCode, entries)
	if err != nil {
		job.Status = domain.ReconJobFailed
		job.Note = err.Error()
		_ = s.store.UpdateJob(ctx, job)
		return nil, err
	}
	for i := range l2 {
		l2[i].JobID = job.JobID
	}
	diffs = append(diffs, l2...)

	l3, err := s.freezeTieOut(ctx, tenantID, assetCode)
	if err != nil {
		job.Status = domain.ReconJobFailed
		job.Note = err.Error()
		_ = s.store.UpdateJob(ctx, job)
		return nil, err
	}
	for i := range l3 {
		l3[i].JobID = job.JobID
	}
	diffs = append(diffs, l3...)

	sum := summarize(len(entries), len(bizLines), diffs, inAmt, outAmt)
	job.Status = domain.ReconJobDone
	job.Summary = sum
	if err := s.store.UpdateJob(ctx, job); err != nil {
		return nil, err
	}
	if err := s.store.CreateDiffs(ctx, diffs); err != nil {
		return nil, err
	}

	sys := sourceSystem
	if sys == "" {
		sys = "all"
	}
	asset := assetCode
	if asset == "" {
		asset = "all"
	}
	return &ReconcileReport{
		Job:           job,
		LedgerEntries: entries,
		Diffs:         diffs,
		Files: map[string]string{
			"recon": reconCSV(sys, asset, date, entries),
			"diff":  diffCSV(sys, asset, date, diffs),
		},
	}, nil
}

func (s *ReconcileService) GetJob(ctx context.Context, jobID string) (*ReconcileReport, error) {
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	diffs, err := s.store.ListDiffs(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return &ReconcileReport{Job: job, Diffs: diffs}, nil
}

func (s *ReconcileService) ReportByDate(ctx context.Context, tenantID, date, sourceSystem, assetCode string) (*ReconcileReport, error) {
	job, err := s.store.LatestJob(ctx, tenantID, date, sourceSystem, assetCode)
	if err != nil {
		return nil, err
	}
	diffs, err := s.store.ListDiffs(ctx, job.JobID)
	if err != nil {
		return nil, err
	}
	return &ReconcileReport{Job: job, Diffs: diffs}, nil
}

func (s *ReconcileService) ResolveDiff(ctx context.Context, diffID, note string) error {
	if diffID == "" {
		return domain.ErrInvalidParam
	}
	return s.store.ResolveDiff(ctx, diffID, note)
}

func MatchBizLines(jobID string, biz []domain.BizLine, entries []*domain.LedgerEntry) []*domain.ReconcileDiff {
	type key struct{ bizNo, cmd string }
	groups := map[key][]*domain.LedgerEntry{}
	for _, e := range entries {
		k := key{e.BizNo, string(e.Command)}
		groups[k] = append(groups[k], e)
	}
	seen := map[key]bool{}
	var diffs []*domain.ReconcileDiff
	for _, line := range biz {
		k := key{line.BizNo, string(line.Command)}
		seen[k] = true
		g := groups[k]
		if len(g) == 0 {
			diffs = append(diffs, newDiff(jobID, domain.DiffMissing, line.BizNo, line.Command, line.AssetCode, line.Amount, 0, ""))
			continue
		}
		assetOK, amountOK, ledgerAmt := false, false, int64(0)
		for _, e := range g {
			if e.AssetCode == line.AssetCode {
				assetOK = true
				ledgerAmt = e.Amount
				if e.Amount == line.Amount {
					amountOK = true
					break
				}
			}
		}
		if !assetOK {
			diffs = append(diffs, newDiff(jobID, domain.DiffAssetMismatch, line.BizNo, line.Command, line.AssetCode, line.Amount, ledgerAmt, g[0].AssetCode))
			continue
		}
		if !amountOK {
			diffs = append(diffs, newDiff(jobID, domain.DiffAmountMismatch, line.BizNo, line.Command, line.AssetCode, line.Amount, ledgerAmt, ""))
		}
	}
	if len(biz) == 0 {
		return diffs
	}
	for k, g := range groups {
		if seen[k] {
			continue
		}
		e := g[0]
		diffs = append(diffs, newDiff(jobID, domain.DiffExtra, e.BizNo, e.Command, e.AssetCode, 0, e.Amount, ""))
	}
	return diffs
}

func (s *ReconcileService) balanceTieOut(ctx context.Context, tenantID, assetCode string, dayEntries []*domain.LedgerEntry) ([]*domain.ReconcileDiff, error) {
	seen := map[string]struct{}{}
	var diffs []*domain.ReconcileDiff
	for _, e := range dayEntries {
		if _, ok := seen[e.AccountID]; ok {
			continue
		}
		seen[e.AccountID] = struct{}{}
		acc, err := s.accs.GetByID(ctx, e.AccountID)
		if err != nil {
			return nil, err
		}
		hist, err := s.entries.ListByAccount(ctx, e.AccountID)
		if err != nil {
			return nil, err
		}
		avail, frozen := reconstruct(hist)
		if avail != acc.Available || frozen != acc.Frozen {
			note := fmt.Sprintf("reconstructed available=%d frozen=%d account available=%d frozen=%d", avail, frozen, acc.Available, acc.Frozen)
			diffs = append(diffs, &domain.ReconcileDiff{
				DiffID:    idgen.New("rd_"),
				Kind:      domain.DiffBalanceTieOut,
				AssetCode: acc.AssetCode,
				AccountID: acc.AccountID,
				Status:    domain.DiffStatusOpen,
				Note:      note,
			})
		}
	}
	_ = tenantID
	_ = assetCode
	return diffs, nil
}

func (s *ReconcileService) freezeTieOut(ctx context.Context, tenantID, assetCode string) ([]*domain.ReconcileDiff, error) {
	accs, err := s.accs.ListByTenant(ctx, tenantID, assetCode)
	if err != nil {
		return nil, err
	}
	frozenList, err := s.freezes.ListFrozen(ctx, tenantID, assetCode)
	if err != nil {
		return nil, err
	}
	sum := map[string]int64{}
	for _, f := range frozenList {
		sum[f.AccountID] += f.Amount
	}
	var diffs []*domain.ReconcileDiff
	for _, acc := range accs {
		got := sum[acc.AccountID]
		if got != acc.Frozen {
			note := fmt.Sprintf("freeze_sum=%d account.frozen=%d", got, acc.Frozen)
			diffs = append(diffs, &domain.ReconcileDiff{
				DiffID:       idgen.New("rd_"),
				Kind:         domain.DiffFreezeTieOut,
				AssetCode:    acc.AssetCode,
				AccountID:    acc.AccountID,
				BizAmount:    got,
				LedgerAmount: acc.Frozen,
				Status:       domain.DiffStatusOpen,
				Note:         note,
			})
		}
	}
	return diffs, nil
}

func reconstruct(entries []*domain.LedgerEntry) (available, frozen int64) {
	for _, e := range entries {
		switch e.Command {
		case domain.CmdCredit:
			available += e.Amount
		case domain.CmdDebit:
			available -= e.Amount
		case domain.CmdFreeze:
			available -= e.Amount
			frozen += e.Amount
		case domain.CmdCapture:
			frozen -= e.Amount
		case domain.CmdRelease:
			frozen -= e.Amount
			available += e.Amount
		case domain.CmdTransfer:
			if e.Direction == domain.DirIN {
				available += e.Amount
			} else {
				available -= e.Amount
			}
		}
	}
	return available, frozen
}

func summarize(ledgerN, bizN int, diffs []*domain.ReconcileDiff, inAmt, outAmt int64) *domain.ReconcileSummary {
	sum := &domain.ReconcileSummary{
		LedgerCount: ledgerN,
		BizCount:    bizN,
		InAmount:    strconv.FormatInt(inAmt, 10),
		OutAmount:   strconv.FormatInt(outAmt, 10),
	}
	for _, d := range diffs {
		switch d.Kind {
		case domain.DiffExtra:
			sum.Extra++
		case domain.DiffMissing:
			sum.Missing++
		case domain.DiffAmountMismatch:
			sum.AmountMismatch++
		case domain.DiffAssetMismatch:
			sum.AssetMismatch++
		case domain.DiffBalanceTieOut:
			sum.BalanceTieOut++
		case domain.DiffFreezeTieOut:
			sum.FreezeTieOut++
		}
	}
	matched := bizN - sum.Missing - sum.AmountMismatch - sum.AssetMismatch
	if matched < 0 {
		matched = 0
	}
	sum.Matched = matched
	return sum
}

func newDiff(jobID, kind, bizNo string, cmd domain.Command, asset string, bizAmt, ledgerAmt int64, note string) *domain.ReconcileDiff {
	return &domain.ReconcileDiff{
		DiffID:       idgen.New("rd_"),
		JobID:        jobID,
		Kind:         kind,
		BizNo:        bizNo,
		Command:      cmd,
		AssetCode:    asset,
		BizAmount:    bizAmt,
		LedgerAmount: ledgerAmt,
		Status:       domain.DiffStatusOpen,
		Note:         note,
	}
}

func reconCSV(sys, asset, date string, entries []*domain.LedgerEntry) string {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"file", "recon_" + sys + "_" + asset + "_" + date + ".csv"})
	_ = w.Write([]string{"biz_no", "command", "asset_code", "amount", "direction", "entry_id"})
	for _, e := range entries {
		_ = w.Write([]string{e.BizNo, string(e.Command), e.AssetCode, strconv.FormatInt(e.Amount, 10), string(e.Direction), e.EntryID})
	}
	w.Flush()
	return buf.String()
}

func diffCSV(sys, asset, date string, diffs []*domain.ReconcileDiff) string {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"file", "diff_" + sys + "_" + asset + "_" + date + ".csv"})
	_ = w.Write([]string{"kind", "biz_no", "command", "asset_code", "biz_amount", "ledger_amount", "account_id", "status", "note"})
	for _, d := range diffs {
		_ = w.Write([]string{d.Kind, d.BizNo, string(d.Command), d.AssetCode, strconv.FormatInt(d.BizAmount, 10), strconv.FormatInt(d.LedgerAmount, 10), d.AccountID, d.Status, d.Note})
	}
	w.Flush()
	return buf.String()
}

