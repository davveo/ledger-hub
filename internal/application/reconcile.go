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
	entries  domain.EntryRepository
	accs     domain.AccountRepository
	freezes  domain.FreezeRepository
	store    domain.ReconcileRepository
	legs     domain.ExchangeLegRepository
	journals domain.JournalRepository
	fx       domain.FxRateRepository
	outDir   string
}

func NewReconcileService(entries domain.EntryRepository, accs domain.AccountRepository, freezes domain.FreezeRepository, store domain.ReconcileRepository) *ReconcileService {
	return &ReconcileService{entries: entries, accs: accs, freezes: freezes, store: store}
}

func (s *ReconcileService) UsePhase3(legs domain.ExchangeLegRepository, journals domain.JournalRepository) *ReconcileService {
	s.legs = legs
	s.journals = journals
	return s
}

func (s *ReconcileService) UseFx(fx domain.FxRateRepository) *ReconcileService {
	s.fx = fx
	return s
}

func (s *ReconcileService) WithOutput(dir string) *ReconcileService {
	s.outDir = dir
	return s
}

type ReconcileReport struct {
	Job           *domain.ReconcileJob    `json:"job"`
	LedgerEntries []*domain.LedgerEntry   `json:"ledger_entries"`
	Diffs         []*domain.ReconcileDiff `json:"diffs"`
	Files         map[string]string       `json:"files"`
}

func (s *ReconcileService) Trigger(ctx context.Context, tenantID, date, sourceSystem, assetCode string, bizLines, channelLines []domain.BizLine) (*ReconcileReport, error) {
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

	l4 := s.fxTieOut(ctx, tenantID, entries)
	for i := range l4 {
		l4[i].JobID = job.JobID
	}
	diffs = append(diffs, l4...)

	l5 := MatchChannelLines(job.JobID, channelLines, entries)
	diffs = append(diffs, l5...)

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
	files := map[string]string{
		"recon":           reconCSV(sys, asset, date, entries),
		"diff":            diffCSV(sys, asset, date, diffs),
		"balance_tie_out": kindCSV("balance_tie_out_"+sys+"_"+asset+"_"+date+".csv", domain.DiffBalanceTieOut, diffs),
		"fx_journal":      fxJournalCSV(sys, asset, date, entries),
	}
	if paths, err := s.writeReconcileFiles(date, sys, asset, files); err == nil {
		for k, p := range paths {
			files[k+"_path"] = p
		}
	}
	return &ReconcileReport{
		Job:           job,
		LedgerEntries: entries,
		Diffs:         diffs,
		Files:         files,
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

func (s *ReconcileService) ListJobs(ctx context.Context, tenantID string, limit int) ([]*domain.ReconcileJob, error) {
	if s == nil || s.store == nil {
		return nil, domain.ErrNotFound
	}
	return s.store.ListJobs(ctx, tenantID, limit)
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

func MatchChannelLines(jobID string, channel []domain.BizLine, entries []*domain.LedgerEntry) []*domain.ReconcileDiff {
	if len(channel) == 0 {
		return nil
	}
	type key struct{ bizNo, asset string }
	credits := map[key]int64{}
	for _, e := range entries {
		if e.Command != domain.CmdCredit || e.HolderType == domain.HolderSystemSubject || e.Direction != domain.DirIN {
			continue
		}
		k := key{e.BizNo, e.AssetCode}
		credits[k] += e.Amount
	}
	var diffs []*domain.ReconcileDiff
	for _, line := range channel {
		asset := line.AssetCode
		if asset == "" {
			asset = "BALANCE_CNY"
		}
		ledgerAmt := credits[key{line.BizNo, asset}]
		if ledgerAmt != line.Amount {
			diffs = append(diffs, newDiff(jobID, domain.DiffChannelMismatch, line.BizNo, domain.CmdCredit, asset, line.Amount, ledgerAmt, "支付渠道金额与 Credit 不一致"))
		}
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

func (s *ReconcileService) fxTieOut(ctx context.Context, tenantID string, entries []*domain.LedgerEntry) []*domain.ReconcileDiff {
	type gkey struct{ journal, biz string }
	groups := map[gkey][]*domain.LedgerEntry{}
	for _, e := range entries {
		if e.Command != domain.CmdExchange {
			continue
		}
		k := gkey{e.JournalID, e.BizNo}
		groups[k] = append(groups[k], e)
	}
	var diffs []*domain.ReconcileDiff
	for k, es := range groups {
		if k.journal == "" {
			diffs = append(diffs, newDiff("", domain.DiffFxIncomplete, k.biz, domain.CmdExchange, "", 0, 0, "缺少 journal_id"))
			continue
		}
		var userOut, userIn, feeAmt int64
		var fromAsset, toAsset, feeAsset string
		assets := map[string]struct{}{}
		for _, e := range es {
			assets[e.AssetCode] = struct{}{}
			if e.HolderType == domain.HolderSystemSubject {
				if e.HolderID == domain.SystemFxFee && e.Direction == domain.DirIN {
					feeAmt += e.Amount
					feeAsset = e.AssetCode
				}
				continue
			}
			if e.Direction == domain.DirOUT {
				userOut += e.Amount
				fromAsset = e.AssetCode
			}
			if e.Direction == domain.DirIN {
				userIn += e.Amount
				toAsset = e.AssetCode
			}
		}
		if userOut == 0 || userIn == 0 || len(assets) < 2 {
			diffs = append(diffs, newDiff("", domain.DiffFxIncomplete, k.biz, domain.CmdExchange, "", userOut, userIn, "兑换分录不完整"))
			continue
		}
		if s.legs == nil {
			continue
		}
		leg, err := s.legs.GetByBizNo(ctx, tenantID, k.biz)
		if err != nil || leg == nil {
			diffs = append(diffs, newDiff("", domain.DiffFxIncomplete, k.biz, domain.CmdExchange, fromAsset, 0, userOut, "缺少 exchange_leg"))
			continue
		}
		if userOut != leg.FromAmount || fromAsset != leg.FromAsset {
			diffs = append(diffs, newDiff("", domain.DiffAmountMismatch, k.biz, domain.CmdExchange, leg.FromAsset, leg.FromAmount, userOut, "from 腿金额/币种与凭证不符"))
		}
		if userIn != leg.ToAmount || toAsset != leg.ToAsset {
			diffs = append(diffs, newDiff("", domain.DiffAmountMismatch, k.biz, domain.CmdExchange, leg.ToAsset, leg.ToAmount, userIn, "to 腿金额/币种与凭证不符"))
		}
		if leg.FeeAmount > 0 && (feeAmt != leg.FeeAmount || (leg.FeeAsset != "" && feeAsset != leg.FeeAsset)) {
			diffs = append(diffs, newDiff("", domain.DiffAmountMismatch, k.biz, domain.CmdExchange, leg.FeeAsset, leg.FeeAmount, feeAmt, "fee 与凭证不符"))
		}
		if leg.RateID != "" && s.fx != nil {
			if _, err := s.fx.Get(ctx, leg.RateID); err != nil {
				diffs = append(diffs, newDiff("", domain.DiffFxIncomplete, k.biz, domain.CmdExchange, "", 0, 0, "缺少 fx_rate "+leg.RateID))
			}
		}
	}
	return diffs
}

func reconstruct(entries []*domain.LedgerEntry) (available, frozen int64) {
	for _, e := range entries {
		switch e.Command {
		case domain.CmdFreeze:
			if e.BizType == "unfreeze" {
				available += e.Amount
				frozen -= e.Amount
				continue
			}
			available -= e.Amount
			frozen += e.Amount
		case domain.CmdCapture:
			frozen -= e.Amount
		case domain.CmdRelease:
			frozen -= e.Amount
			available += e.Amount
		case domain.CmdReverse:
			if e.BizType == "unfreeze" {
				available += e.Amount
				frozen -= e.Amount
				continue
			}
			if e.Direction == domain.DirIN {
				available += e.Amount
			} else {
				available -= e.Amount
			}
		default:
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
		case domain.DiffFxIncomplete:
			sum.FxIncomplete++
		case domain.DiffChannelMismatch:
			sum.ChannelMismatch++
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

func kindCSV(filename, kind string, diffs []*domain.ReconcileDiff) string {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"file", filename})
	_ = w.Write([]string{"kind", "biz_no", "command", "asset_code", "biz_amount", "ledger_amount", "account_id", "status", "note"})
	for _, d := range diffs {
		if d.Kind != kind {
			continue
		}
		_ = w.Write([]string{d.Kind, d.BizNo, string(d.Command), d.AssetCode, strconv.FormatInt(d.BizAmount, 10), strconv.FormatInt(d.LedgerAmount, 10), d.AccountID, d.Status, d.Note})
	}
	w.Flush()
	return buf.String()
}

func fxJournalCSV(sys, asset, date string, entries []*domain.LedgerEntry) string {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"file", "fx_journal_" + sys + "_" + asset + "_" + date + ".csv"})
	_ = w.Write([]string{"biz_no", "journal_id", "asset_code", "holder_type", "holder_id", "direction", "amount", "entry_id"})
	for _, e := range entries {
		if e.Command != domain.CmdExchange {
			continue
		}
		_ = w.Write([]string{e.BizNo, e.JournalID, e.AssetCode, string(e.HolderType), e.HolderID, string(e.Direction), strconv.FormatInt(e.Amount, 10), e.EntryID})
	}
	w.Flush()
	return buf.String()
}

