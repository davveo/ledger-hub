package application

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
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

type reconPayload struct {
	BizLines     []domain.BizLine `json:"biz_lines,omitempty"`
	ChannelLines []domain.BizLine `json:"channel_lines,omitempty"`
}

func reconKey(source, asset, jobType string) (string, string, string) {
	if jobType == "" {
		jobType = domain.ReconJobTypeDaily
	}
	return source, asset, jobType
}

func reconReusable(status string) bool {
	return status == domain.ReconJobQueued || status == domain.ReconJobRunning || status == domain.ReconJobDone
}

func (s *ReconcileService) Enqueue(ctx context.Context, tenantID, date, sourceSystem, assetCode, jobType string, forceNew bool, bizLines, channelLines []domain.BizLine) (*domain.ReconcileJob, error) {
	if tenantID == "" || date == "" {
		return nil, domain.ErrInvalidParam
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, domain.Keyed(domain.CodeDateNotISO, domain.KeyDateNotISO)
	}
	sourceSystem, assetCode, jobType = reconKey(sourceSystem, assetCode, jobType)
	latest, err := s.store.FindJobByKey(ctx, tenantID, date, sourceSystem, assetCode, jobType)
	if err != nil && !domain.Is(err, domain.CodeNotFound) {
		return nil, err
	}
	version := 1
	if latest != nil {
		if !forceNew && reconReusable(latest.Status) {
			return latest, nil
		}
		version = latest.Version + 1
	}
	payload, _ := json.Marshal(reconPayload{BizLines: bizLines, ChannelLines: channelLines})
	job := &domain.ReconcileJob{
		JobID:        idgen.New("rj_"),
		TenantID:     tenantID,
		Date:         date,
		SourceSystem: sourceSystem,
		AssetCode:    assetCode,
		JobType:      jobType,
		Version:      version,
		Status:       domain.ReconJobQueued,
		Phase:        "queued",
		PayloadJSON:  string(payload),
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.store.CreateJob(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *ReconcileService) DrainQueued(ctx context.Context, limit int) (int, error) {
	if s == nil || s.store == nil {
		return 0, nil
	}
	list, err := s.store.ListQueuedJobs(ctx, limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, job := range list {
		if _, err := s.RunJob(ctx, job.JobID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *ReconcileService) Rerun(ctx context.Context, tenantID, jobID string) (*domain.ReconcileJob, error) {
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if err := tenantMatch(job.TenantID, tenantID); err != nil {
		return nil, err
	}
	var p reconPayload
	_ = json.Unmarshal([]byte(job.PayloadJSON), &p)
	return s.Enqueue(ctx, job.TenantID, job.Date, job.SourceSystem, job.AssetCode, job.JobType, true, p.BizLines, p.ChannelLines)
}

func (s *ReconcileService) Trigger(ctx context.Context, tenantID, date, sourceSystem, assetCode string, bizLines, channelLines []domain.BizLine) (*ReconcileReport, error) {
	job, err := s.Enqueue(ctx, tenantID, date, sourceSystem, assetCode, domain.ReconJobTypeDaily, false, bizLines, channelLines)
	if err != nil {
		return nil, err
	}
	if job.Status == domain.ReconJobDone || job.Status == domain.ReconJobRunning {
		return s.GetJob(ctx, tenantID, job.JobID)
	}
	return s.RunJob(ctx, job.JobID)
}

func (s *ReconcileService) RunJob(ctx context.Context, jobID string) (*ReconcileReport, error) {
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status == domain.ReconJobDone {
		return s.GetJob(ctx, job.TenantID, job.JobID)
	}
	var p reconPayload
	_ = json.Unmarshal([]byte(job.PayloadJSON), &p)
	day, err := time.Parse("2006-01-02", job.Date)
	if err != nil {
		job.Status = domain.ReconJobFailed
		job.Note = "date 需为 YYYY-MM-DD"
		_ = s.store.UpdateJob(ctx, job)
		return nil, domain.Keyed(domain.CodeDateNotISO, domain.KeyDateNotISO)
	}
	from := day.UTC()
	to := from.Add(24 * time.Hour)
	job.Status = domain.ReconJobRunning
	job.Phase = "matching"
	if err := s.store.UpdateJob(ctx, job); err != nil {
		return nil, err
	}

	entries, err := s.entries.ListByRange(ctx, job.TenantID, job.SourceSystem, job.AssetCode, from, to)
	if err != nil {
		job.Status = domain.ReconJobFailed
		job.Phase = "failed"
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

	diffs := MatchBizLines(job.JobID, p.BizLines, entries)
	l2, err := s.balanceTieOut(ctx, job.TenantID, job.AssetCode, entries)
	if err != nil {
		job.Status = domain.ReconJobFailed
		job.Phase = "failed"
		job.Note = err.Error()
		_ = s.store.UpdateJob(ctx, job)
		return nil, err
	}
	for i := range l2 {
		l2[i].JobID = job.JobID
	}
	diffs = append(diffs, l2...)

	l3, err := s.freezeTieOut(ctx, job.TenantID, job.AssetCode)
	if err != nil {
		job.Status = domain.ReconJobFailed
		job.Phase = "failed"
		job.Note = err.Error()
		_ = s.store.UpdateJob(ctx, job)
		return nil, err
	}
	for i := range l3 {
		l3[i].JobID = job.JobID
	}
	diffs = append(diffs, l3...)

	if lPend := s.pendingSettlementTieOut(ctx, job.TenantID, job.AssetCode); len(lPend) > 0 {
		for i := range lPend {
			lPend[i].JobID = job.JobID
		}
		diffs = append(diffs, lPend...)
	}

	l4 := s.fxTieOut(ctx, job.TenantID, entries)
	for i := range l4 {
		l4[i].JobID = job.JobID
	}
	diffs = append(diffs, l4...)

	l5 := MatchChannelLines(job.JobID, p.ChannelLines, entries)
	diffs = append(diffs, l5...)

	sum := summarize(len(entries), len(p.BizLines), diffs, inAmt, outAmt)
	job.Status = domain.ReconJobDone
	job.Phase = "done"
	job.Summary = sum
	if err := s.store.UpdateJob(ctx, job); err != nil {
		return nil, err
	}
	if err := s.store.CreateDiffs(ctx, diffs); err != nil {
		return nil, err
	}

	sys := job.SourceSystem
	if sys == "" {
		sys = "all"
	}
	asset := job.AssetCode
	if asset == "" {
		asset = "all"
	}
	files := map[string]string{
		"recon":           reconCSV(sys, asset, job.Date, entries),
		"diff":            diffCSV(sys, asset, job.Date, diffs),
		"balance_tie_out": kindCSV("balance_tie_out_"+sys+"_"+asset+"_"+job.Date+".csv", domain.DiffBalanceTieOut, diffs),
		"fx_journal":      fxJournalCSV(sys, asset, job.Date, entries),
	}
	if paths, err := s.writeReconcileFiles(job.Date, sys, asset, files); err == nil {
		for k, pth := range paths {
			files[k+"_path"] = pth
		}
	}
	return &ReconcileReport{
		Job:           job,
		LedgerEntries: entries,
		Diffs:         diffs,
		Files:         files,
	}, nil
}

func (s *ReconcileService) GetJob(ctx context.Context, tenantID, jobID string) (*ReconcileReport, error) {
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if err := tenantMatch(job.TenantID, tenantID); err != nil {
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

func (s *ReconcileService) ListOpenDiffs(ctx context.Context, tenantID string, limit int) ([]*domain.ReconcileDiff, error) {
	if s == nil || s.store == nil {
		return nil, domain.ErrNotFound
	}
	return s.store.ListOpenDiffs(ctx, tenantID, limit)
}

func (s *ReconcileService) ResolveDiff(ctx context.Context, tenantID, diffID, note, operator string) error {
	d, err := s.diffInTenant(ctx, tenantID, diffID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if operator != "" && note != "" {
		d.Note = operator + ": " + note
	} else if operator != "" {
		d.Note = operator
	} else {
		d.Note = note
	}
	d.Status = domain.DiffStatusResolved
	d.ResolvedBy = operator
	d.ClosedBy = operator
	d.ClosedAt = &now
	if err := s.store.UpdateDiff(ctx, d); err != nil {
		return err
	}
	return s.store.CreateDiffEvent(ctx, &domain.ReconcileDiffEvent{
		DiffID:   diffID,
		Action:   "resolve",
		Operator: operator,
		Detail:   note,
	})
}

func (s *ReconcileService) AssignDiff(ctx context.Context, tenantID, diffID, assignee, note, operator string) (*domain.ReconcileDiff, error) {
	d, err := s.diffInTenant(ctx, tenantID, diffID)
	if err != nil {
		return nil, err
	}
	if assignee == "" {
		return nil, domain.ErrInvalidParam
	}
	d.Assignee = assignee
	if note != "" {
		d.Note = note
	}
	if err := s.store.UpdateDiff(ctx, d); err != nil {
		return nil, err
	}
	_ = s.store.CreateDiffEvent(ctx, &domain.ReconcileDiffEvent{
		DiffID:   diffID,
		Action:   "assign",
		Operator: operator,
		Detail:   assignee + " " + note,
	})
	return d, nil
}

func (s *ReconcileService) ListDiffEvents(ctx context.Context, tenantID, diffID string) ([]*domain.ReconcileDiffEvent, error) {
	if _, err := s.diffInTenant(ctx, tenantID, diffID); err != nil {
		return nil, err
	}
	return s.store.ListDiffEvents(ctx, diffID)
}

func (s *ReconcileService) diffInTenant(ctx context.Context, tenantID, diffID string) (*domain.ReconcileDiff, error) {
	if diffID == "" {
		return nil, domain.ErrInvalidParam
	}
	d, err := s.store.GetDiff(ctx, diffID)
	if err != nil {
		return nil, err
	}
	job, err := s.store.GetJob(ctx, d.JobID)
	if err != nil {
		return nil, err
	}
	if err := tenantMatch(job.TenantID, tenantID); err != nil {
		return nil, err
	}
	return d, nil
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

func (s *ReconcileService) pendingSettlementTieOut(ctx context.Context, tenantID, assetCode string) []*domain.ReconcileDiff {
	if s.accs == nil || assetCode == "" {
		return nil
	}
	acc, err := s.accs.Get(ctx, tenantID, domain.Holder{Type: domain.HolderSystemSubject, ID: domain.SystemPendingSettlement}, assetCode)
	if err != nil || acc.Available == 0 {
		return nil
	}
	return []*domain.ReconcileDiff{{
		DiffID:       idgen.New("rd_"),
		Kind:         domain.DiffCrossShardInFlight,
		AssetCode:    assetCode,
		AccountID:    acc.AccountID,
		LedgerAmount: acc.Available,
		Status:       domain.DiffStatusOpen,
		Note:         "pending_settlement 存在在途余额，请检查未完成跨分片 Saga",
	}}
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
			snap, err := s.fx.Get(ctx, leg.RateID)
			if err != nil || snap.TenantID != tenantID {
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

