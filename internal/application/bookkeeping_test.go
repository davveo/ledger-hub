package application

import (
	"context"
	"testing"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

type memTx struct{}

func (memTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

type mem struct {
	assets  map[string]*domain.Asset
	accs    map[string]*domain.Account
	byKey   map[string]*domain.Account
	entries []*domain.LedgerEntry
	freezes map[string]*domain.FreezeOrder
	fzBiz   map[string]*domain.FreezeOrder
	idem    map[string]*domain.IdempotencyRecord
}

func newMem() *mem {
	return &mem{
		assets:  map[string]*domain.Asset{},
		accs:    map[string]*domain.Account{},
		byKey:   map[string]*domain.Account{},
		freezes: map[string]*domain.FreezeOrder{},
		fzBiz:   map[string]*domain.FreezeOrder{},
		idem:    map[string]*domain.IdempotencyRecord{},
	}
}

func accKey(tenant string, h domain.Holder, asset string) string {
	return tenant + "|" + string(h.Type) + "|" + h.ID + "|" + asset
}

func (m *mem) Save(_ context.Context, a *domain.Asset) error {
	m.assets[a.TenantID+"|"+a.AssetCode] = a
	return nil
}
func (m *mem) Get(_ context.Context, tenant, code string) (*domain.Asset, error) {
	a := m.assets[tenant+"|"+code]
	if a == nil {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}
func (m *mem) List(_ context.Context, tenant string) ([]*domain.Asset, error) {
	var out []*domain.Asset
	for _, a := range m.assets {
		if a.TenantID == tenant {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out, nil
}

type memAccount struct{ m *mem }

func (r memAccount) GetByID(ctx context.Context, id string) (*domain.Account, error) {
	a := r.m.accs[id]
	if a == nil {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}
func (r memAccount) Get(_ context.Context, tenant string, h domain.Holder, asset string) (*domain.Account, error) {
	a := r.m.byKey[accKey(tenant, h, asset)]
	if a == nil {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}
func (r memAccount) GetForUpdate(ctx context.Context, tenant string, h domain.Holder, asset string) (*domain.Account, error) {
	return r.Get(ctx, tenant, h, asset)
}
func (r memAccount) Create(_ context.Context, a *domain.Account) error {
	cp := *a
	r.m.accs[a.AccountID] = &cp
	r.m.byKey[accKey(a.TenantID, domain.Holder{Type: a.HolderType, ID: a.HolderID}, a.AssetCode)] = &cp
	return nil
}
func (r memAccount) UpdateBalances(_ context.Context, a *domain.Account) error {
	cur := r.m.accs[a.AccountID]
	if cur == nil || cur.Version != a.Version {
		return domain.NewError(domain.CodeInternal, "lock")
	}
	cur.Available, cur.Frozen, cur.Version = a.Available, a.Frozen, a.Version+1
	a.Version = cur.Version
	return nil
}
func (r memAccount) ListByTenant(_ context.Context, tenant, asset string) ([]*domain.Account, error) {
	var out []*domain.Account
	for _, a := range r.m.accs {
		if a.TenantID != tenant {
			continue
		}
		if asset != "" && a.AssetCode != asset {
			continue
		}
		cp := *a
		out = append(out, &cp)
	}
	return out, nil
}
func (r memAccount) ListByHolder(_ context.Context, tenant string, h domain.Holder, asset string) ([]*domain.Account, error) {
	var out []*domain.Account
	for _, a := range r.m.accs {
		if a.TenantID != tenant || a.HolderID != h.ID {
			continue
		}
		if h.Type != "" && a.HolderType != h.Type {
			continue
		}
		if asset != "" && a.AssetCode != asset {
			continue
		}
		cp := *a
		out = append(out, &cp)
	}
	return out, nil
}
func (r memAccount) UpdateStatus(_ context.Context, a *domain.Account) error {
	cur := r.m.accs[a.AccountID]
	if cur == nil {
		return domain.ErrNotFound
	}
	cur.Status = a.Status
	return nil
}

func (m *mem) CreateEntry(_ context.Context, e *domain.LedgerEntry) error {
	cp := *e
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	m.entries = append(m.entries, &cp)
	return nil
}
func (m *mem) ListByBizNo(_ context.Context, _, biz string) ([]*domain.LedgerEntry, error) {
	var out []*domain.LedgerEntry
	for _, e := range m.entries {
		if e.BizNo == biz {
			out = append(out, e)
		}
	}
	return out, nil
}
func (m *mem) ListByHolder(_ context.Context, tenant string, h domain.Holder, asset string, _, _ *time.Time, page domain.Page) ([]*domain.LedgerEntry, error) {
	page = page.Clamp(50, 200)
	var all []*domain.LedgerEntry
	for _, e := range m.entries {
		if e.HolderID != h.ID {
			continue
		}
		if tenant != "" && e.TenantID != tenant {
			continue
		}
		if h.Type != "" && e.HolderType != h.Type {
			continue
		}
		if asset != "" && e.AssetCode != asset {
			continue
		}
		all = append(all, e)
	}
	if page.Offset >= len(all) {
		return nil, nil
	}
	end := page.Offset + page.Limit
	if end > len(all) {
		end = len(all)
	}
	return all[page.Offset:end], nil
}
func (m *mem) ListByRange(_ context.Context, _, _, _ string, _, _ time.Time) ([]*domain.LedgerEntry, error) {
	return m.entries, nil
}
func (m *mem) ListByAccount(_ context.Context, id string) ([]*domain.LedgerEntry, error) {
	var out []*domain.LedgerEntry
	for _, e := range m.entries {
		if e.AccountID == id {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *mem) CreateFreeze(_ context.Context, f *domain.FreezeOrder) error {
	cp := *f
	m.freezes[f.FreezeID] = &cp
	m.fzBiz[f.TenantID+"|"+f.BizNo] = &cp
	return nil
}
func (m *mem) GetFreezeByID(_ context.Context, id string) (*domain.FreezeOrder, error) {
	f := m.freezes[id]
	if f == nil {
		return nil, domain.ErrNotFound
	}
	cp := *f
	return &cp, nil
}
func (m *mem) GetFreezeByBizNo(_ context.Context, tenant, biz string) (*domain.FreezeOrder, error) {
	f := m.fzBiz[tenant+"|"+biz]
	if f == nil {
		return nil, domain.ErrNotFound
	}
	cp := *f
	return &cp, nil
}
func (m *mem) UpdateStatus(_ context.Context, id string, from, to domain.FreezeStatus) error {
	f := m.freezes[id]
	if f == nil || f.Status != from {
		return domain.ErrFreezeStateInvalid
	}
	f.Status = to
	return nil
}
func (m *mem) ListExpired(_ context.Context, now time.Time, limit int) ([]*domain.FreezeOrder, error) {
	var out []*domain.FreezeOrder
	for _, f := range m.freezes {
		if f.Status != domain.FreezeFrozen || f.ExpireAt == nil || f.ExpireAt.After(now) {
			continue
		}
		cp := *f
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
func (m *mem) ListFrozen(_ context.Context, tenant, asset string) ([]*domain.FreezeOrder, error) {
	var out []*domain.FreezeOrder
	for _, f := range m.freezes {
		if f.Status != domain.FreezeFrozen {
			continue
		}
		if tenant != "" && f.TenantID != tenant {
			continue
		}
		if asset != "" && f.AssetCode != asset {
			continue
		}
		cp := *f
		out = append(out, &cp)
	}
	return out, nil
}

func (m *mem) GetIdem(_ context.Context, tenant, src, biz string, cmd domain.Command) (*domain.IdempotencyRecord, error) {
	return m.idem[tenant+"|"+src+"|"+biz+"|"+string(cmd)], nil
}
func (m *mem) CreateIdem(_ context.Context, rec *domain.IdempotencyRecord) error {
	m.idem[rec.TenantID+"|"+rec.SourceSystem+"|"+rec.BizNo+"|"+string(rec.Command)] = rec
	return nil
}

type memEntry struct{ m *mem }
type memFreeze struct{ m *mem }
type memIdem struct{ m *mem }

func (r memEntry) Create(ctx context.Context, e *domain.LedgerEntry) error {
	return r.m.CreateEntry(ctx, e)
}
func (r memEntry) ListByBizNo(ctx context.Context, t, b string) ([]*domain.LedgerEntry, error) {
	return r.m.ListByBizNo(ctx, t, b)
}
func (r memEntry) ListByHolder(ctx context.Context, t string, h domain.Holder, a string, from, to *time.Time, page domain.Page) ([]*domain.LedgerEntry, error) {
	return r.m.ListByHolder(ctx, t, h, a, from, to, page)
}
func (r memEntry) ListByRange(ctx context.Context, t, s, a string, from, to time.Time) ([]*domain.LedgerEntry, error) {
	return r.m.ListByRange(ctx, t, s, a, from, to)
}
func (r memEntry) ListByAccount(ctx context.Context, id string) ([]*domain.LedgerEntry, error) {
	return r.m.ListByAccount(ctx, id)
}
func (r memEntry) ListByJournal(_ context.Context, journalID string) ([]*domain.LedgerEntry, error) {
	var out []*domain.LedgerEntry
	for _, e := range r.m.entries {
		if e.JournalID == journalID {
			out = append(out, e)
		}
	}
	return out, nil
}
func (r memFreeze) Create(ctx context.Context, f *domain.FreezeOrder) error {
	return r.m.CreateFreeze(ctx, f)
}
func (r memFreeze) GetByID(ctx context.Context, id string) (*domain.FreezeOrder, error) {
	return r.m.GetFreezeByID(ctx, id)
}
func (r memFreeze) GetByBizNo(ctx context.Context, t, b string) (*domain.FreezeOrder, error) {
	return r.m.GetFreezeByBizNo(ctx, t, b)
}
func (r memFreeze) UpdateStatus(ctx context.Context, id string, from, to domain.FreezeStatus) error {
	return r.m.UpdateStatus(ctx, id, from, to)
}
func (r memFreeze) Update(_ context.Context, f *domain.FreezeOrder) error {
	cur := r.m.freezes[f.FreezeID]
	if cur == nil {
		return domain.ErrNotFound
	}
	cur.Amount = f.Amount
	cur.Status = f.Status
	cur.ExpireAt = f.ExpireAt
	r.m.fzBiz[f.TenantID+"|"+f.BizNo] = cur
	return nil
}
func (r memFreeze) ListExpired(ctx context.Context, n time.Time, l int) ([]*domain.FreezeOrder, error) {
	return r.m.ListExpired(ctx, n, l)
}
func (r memFreeze) ListFrozen(ctx context.Context, t, a string) ([]*domain.FreezeOrder, error) {
	return r.m.ListFrozen(ctx, t, a)
}
func (r memFreeze) ListByHolder(_ context.Context, tenant string, h domain.Holder, asset, status string, page domain.Page) ([]*domain.FreezeOrder, error) {
	page = page.Clamp(50, 200)
	accIDs := map[string]struct{}{}
	for _, a := range r.m.accs {
		if a.TenantID == tenant && a.HolderID == h.ID && (h.Type == "" || a.HolderType == h.Type) && (asset == "" || a.AssetCode == asset) {
			accIDs[a.AccountID] = struct{}{}
		}
	}
	var out []*domain.FreezeOrder
	for _, f := range r.m.freezes {
		if _, ok := accIDs[f.AccountID]; !ok {
			continue
		}
		if status != "" && string(f.Status) != status {
			continue
		}
		cp := *f
		out = append(out, &cp)
	}
	if page.Offset >= len(out) {
		return nil, nil
	}
	end := page.Offset + page.Limit
	if end > len(out) {
		end = len(out)
	}
	return out[page.Offset:end], nil
}
func (r memIdem) Get(ctx context.Context, t, s, b string, c domain.Command) (*domain.IdempotencyRecord, error) {
	return r.m.GetIdem(ctx, t, s, b, c)
}
func (r memIdem) Create(ctx context.Context, rec *domain.IdempotencyRecord) error {
	return r.m.CreateIdem(ctx, rec)
}
func (r memIdem) DeleteBefore(_ context.Context, before time.Time) (int64, error) {
	var n int64
	for k, rec := range r.m.idem {
		_ = rec
		_ = before
		delete(r.m.idem, k)
		n++
	}
	return n, nil
}

func setupBooks(t *testing.T) (*Bookkeeping, *mem) {
	t.Helper()
	st := newMem()
	_ = st.Save(context.Background(), &domain.Asset{
		TenantID: "t_default", AssetCode: "POINT", Name: "积分", Status: domain.AssetActive, FreezeSupported: true,
	})
	acl := NewACL([]domain.ACLRule{
		{SourceSystem: "campaign", Commands: []string{"Credit"}, Assets: []string{"POINT"}},
		{SourceSystem: "order", Commands: []string{"Freeze", "Capture", "Release"}, Assets: []string{"POINT", "BALANCE_CNY"}},
		{SourceSystem: "pay", Commands: []string{"Credit"}, Assets: []string{"BALANCE_CNY"}},
		{SourceSystem: "wallet", Commands: []string{"Transfer", "Credit", "Debit", "Freeze", "Capture", "Release", "Exchange", "Reverse"}, Assets: []string{"POINT", "BALANCE_CNY", "BALANCE_USD"}},
		{SourceSystem: "worker", Commands: []string{"Release", "Debit", "Transfer", "Reverse"}, Assets: []string{"*"}},
	})
	b := NewBookkeeping(memTx{}, st, memAccount{st}, memEntry{st}, memFreeze{st}, memIdem{st}, acl)
	return b, st
}

func TestCreditDebitFreezeCaptureRelease(t *testing.T) {
	ctx := context.Background()
	b, _ := setupBooks(t)
	holder := domain.Holder{Type: domain.HolderUser, ID: "u1"}

	credit, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "campaign",
		BizNo: "campaign:c1", Holder: holder, AssetCode: "POINT", Amount: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if credit.Account.Available != "100" {
		t.Fatalf("available=%s", credit.Account.Available)
	}

	fz, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdFreeze, TenantID: "t_default", SourceSystem: "order",
		BizNo: "order:freeze:O1", Holder: holder, AssetCode: "POINT", Amount: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fz.Account.Available != "60" || fz.Account.Frozen != "40" {
		t.Fatalf("after freeze %+v", fz.Account)
	}

	cap, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCapture, TenantID: "t_default", SourceSystem: "order",
		BizNo: "order:capture:O1", FreezeID: fz.FreezeID, AssetCode: "POINT",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cap.Account.Available != "60" || cap.Account.Frozen != "0" {
		t.Fatalf("after capture %+v", cap.Account)
	}

	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCapture, TenantID: "t_default", SourceSystem: "order",
		BizNo: "order:capture:O1", FreezeID: fz.FreezeID, AssetCode: "POINT",
	}); err != nil {
		t.Fatal("repeat capture should be idempotent", err)
	}

	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "order",
		BizNo: "order:bad", Holder: holder, AssetCode: "POINT", Amount: 1,
	}); !domain.Is(err, domain.CodeForbidden) {
		t.Fatalf("order should not Credit, err=%v", err)
	}
}

func TestTransferSameAsset(t *testing.T) {
	ctx := context.Background()
	b, _ := setupBooks(t)
	from := domain.Holder{Type: domain.HolderUser, ID: "u1"}
	to := domain.Holder{Type: domain.HolderUser, ID: "u2"}
	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:in", Holder: from, AssetCode: "POINT", Amount: 30,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdTransfer, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:tf1", Holder: from, ToHolder: &to, AssetCode: "POINT", Amount: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.JournalID == "" || res.Account.Available != "20" || res.ToAccount.Available != "10" {
		t.Fatalf("transfer result %+v", res)
	}
}

func TestIdempotentCredit(t *testing.T) {
	ctx := context.Background()
	b, _ := setupBooks(t)
	req := domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "campaign",
		BizNo: "campaign:same", Holder: domain.Holder{Type: domain.HolderUser, ID: "u1"}, AssetCode: "POINT", Amount: 5,
	}
	r1, err := b.Execute(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := b.Execute(ctx, req)
	if err != nil || !r2.IdempotentReplay || r2.Account.Available != r1.Account.Available {
		t.Fatalf("replay %+v err=%v", r2, err)
	}
	req.Amount = 9
	if _, err := b.Execute(ctx, req); !domain.Is(err, domain.CodeIdempotencyConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestExchangeWithRateAndFee(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	_ = st.Save(ctx, &domain.Asset{
		TenantID: "t_default", AssetCode: "BALANCE_CNY", Name: "人民币", Precision: 2, Status: domain.AssetActive,
	})
	_ = st.Save(ctx, &domain.Asset{
		TenantID: "t_default", AssetCode: "BALANCE_USD", Name: "美元", Precision: 2, Status: domain.AssetActive,
	})
	holder := domain.Holder{Type: domain.HolderUser, ID: "u_fx"}
	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:cny", Holder: holder, AssetCode: "BALANCE_CNY", Amount: 20000,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdExchange, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:fx:1", Holder: holder, AssetCode: "BALANCE_CNY", Amount: 10000,
		ToAssetCode: "BALANCE_USD", ToAmount: 1400, FeeAsset: "BALANCE_CNY", FeeAmount: 10,
		Fx: &domain.FxQuote{Rate: "0.14000000", BaseAsset: "BALANCE_CNY", QuoteAsset: "BALANCE_USD"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.JournalID == "" || res.ToAccount == nil || res.ToAccount.Available != "1400" {
		t.Fatalf("exchange result %+v", res)
	}
	if res.Account.Available != "9990" {
		t.Fatalf("cny after fx=%s", res.Account.Available)
	}
	var userIn, userOut, feeIn, clr bool
	for _, e := range st.entries {
		if e.JournalID != res.JournalID {
			continue
		}
		if e.HolderType != domain.HolderSystemSubject && e.Direction == domain.DirOUT && e.AssetCode == "BALANCE_CNY" && e.Amount == 10000 {
			userOut = true
		}
		if e.HolderType != domain.HolderSystemSubject && e.Direction == domain.DirIN && e.AssetCode == "BALANCE_USD" {
			userIn = true
		}
		if e.HolderID == domain.SystemFxFee && e.Direction == domain.DirIN {
			feeIn = true
		}
		if e.HolderID == domain.SystemFxClearing {
			clr = true
		}
	}
	if !userIn || !userOut || !feeIn || !clr {
		t.Fatalf("incomplete journal entries userIn=%v userOut=%v feeIn=%v clr=%v", userIn, userOut, feeIn, clr)
	}
}

func TestExchangeSlippageRejected(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	_ = st.Save(ctx, &domain.Asset{TenantID: "t_default", AssetCode: "BALANCE_CNY", Name: "人民币", Precision: 2, Status: domain.AssetActive})
	_ = st.Save(ctx, &domain.Asset{TenantID: "t_default", AssetCode: "BALANCE_USD", Name: "美元", Precision: 2, Status: domain.AssetActive})
	holder := domain.Holder{Type: domain.HolderUser, ID: "u_fx2"}
	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:cny2", Holder: holder, AssetCode: "BALANCE_CNY", Amount: 20000,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdExchange, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:fx:bad", Holder: holder, AssetCode: "BALANCE_CNY", Amount: 10000,
		ToAssetCode: "BALANCE_USD", ToAmount: 2000,
		Fx: &domain.FxQuote{Rate: "0.14000000"},
	})
	if !domain.Is(err, domain.CodeSlippage) {
		t.Fatalf("want slippage, got %v", err)
	}
}

func TestCrossShardTransferRejected(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	b.UsePhase3(nil, nil, nil, nil, func(a, b string) bool { return a == b })
	from := domain.Holder{Type: domain.HolderUser, ID: "u1"}
	to := domain.Holder{Type: domain.HolderUser, ID: "u2"}
	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:in2", Holder: from, AssetCode: "POINT", Amount: 30,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdTransfer, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:tf-cross", Holder: from, ToHolder: &to, AssetCode: "POINT", Amount: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Account.Available != "20" || res.ToAccount.Available != "10" {
		t.Fatalf("cross shard result %+v", res)
	}
	var pendingIn, pendingOut bool
	for _, e := range st.entries {
		if e.HolderID != domain.SystemPendingSettlement {
			continue
		}
		if e.Direction == domain.DirIN && e.Amount == 10 {
			pendingIn = true
		}
		if e.Direction == domain.DirOUT && e.Amount == 10 {
			pendingOut = true
		}
	}
	if !pendingIn || !pendingOut {
		t.Fatalf("want pending_settlement both legs, in=%v out=%v", pendingIn, pendingOut)
	}
}

func TestReverseCredit(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	holder := domain.Holder{Type: domain.HolderUser, ID: "u_rev"}
	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:rev-src", Holder: holder, AssetCode: "POINT", Amount: 50,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdReverse, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:rev-1", RelatedBizNo: "wallet:rev-src",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.JournalID == "" {
		t.Fatal("want reverse journal")
	}
	acc, err := memAccount{st}.Get(ctx, "t_default", holder, "POINT")
	if err != nil {
		t.Fatal(err)
	}
	if acc.Available != 0 {
		t.Fatalf("available=%d", acc.Available)
	}
}

func TestPartialCapture(t *testing.T) {
	ctx := context.Background()
	b, _ := setupBooks(t)
	holder := domain.Holder{Type: domain.HolderUser, ID: "u_cap"}
	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "campaign",
		BizNo: "campaign:pc", Holder: holder, AssetCode: "POINT", Amount: 100,
	}); err != nil {
		t.Fatal(err)
	}
	fz, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdFreeze, TenantID: "t_default", SourceSystem: "order",
		BizNo: "order:freeze:pc", Holder: holder, AssetCode: "POINT", Amount: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	part, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCapture, TenantID: "t_default", SourceSystem: "order",
		BizNo: "order:capture:pc1", FreezeID: fz.FreezeID, Amount: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if part.Account.Available != "60" || part.Account.Frozen != "30" {
		t.Fatalf("partial capture %+v", part.Account)
	}
	rest, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCapture, TenantID: "t_default", SourceSystem: "order",
		BizNo: "order:capture:pc2", FreezeID: fz.FreezeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rest.Account.Available != "60" || rest.Account.Frozen != "0" {
		t.Fatalf("rest capture %+v", rest.Account)
	}
}

func TestDisabledAccountRejected(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	holder := domain.Holder{Type: domain.HolderUser, ID: "u_off"}
	res, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "campaign",
		BizNo: "campaign:off1", Holder: holder, AssetCode: "POINT", Amount: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	st.accs[res.Account.AccountID].Status = domain.AccountDisabled
	_, err = b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "campaign",
		BizNo: "campaign:off2", Holder: holder, AssetCode: "POINT", Amount: 1,
	})
	if !domain.Is(err, domain.CodeInvalidParam) {
		t.Fatalf("want disabled account, got %v", err)
	}
}

func TestHolderTypesRejected(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	_ = st.Save(ctx, &domain.Asset{
		TenantID: "t_default", AssetCode: "POINT", Name: "积分", Status: domain.AssetActive,
		HolderTypes: []string{"merchant"}, FreezeSupported: true,
	})
	_, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "campaign",
		BizNo: "campaign:ht", Holder: domain.Holder{Type: domain.HolderUser, ID: "u1"}, AssetCode: "POINT", Amount: 1,
	})
	if !domain.Is(err, domain.CodeInvalidParam) {
		t.Fatalf("want holder type rejected, got %v", err)
	}
}

type memSaga struct {
	byID  map[string]*domain.TransferSaga
	byBiz map[string]*domain.TransferSaga
}

func newMemSaga() *memSaga {
	return &memSaga{byID: map[string]*domain.TransferSaga{}, byBiz: map[string]*domain.TransferSaga{}}
}

func sagaBizKey(tenant, src, biz string) string { return tenant + "|" + src + "|" + biz }

func (m *memSaga) Create(_ context.Context, s *domain.TransferSaga) error {
	if s == nil || s.SagaID == "" {
		return domain.ErrInvalidParam
	}
	key := sagaBizKey(s.TenantID, s.SourceSystem, s.BizNo)
	if _, ok := m.byBiz[key]; ok {
		return domain.ErrIdempotencyConflict
	}
	cp := *s
	m.byID[s.SagaID] = &cp
	m.byBiz[key] = &cp
	return nil
}

func (m *memSaga) Update(_ context.Context, s *domain.TransferSaga) error {
	cur := m.byID[s.SagaID]
	if cur == nil {
		return domain.ErrNotFound
	}
	*cur = *s
	return nil
}

func (m *memSaga) Get(_ context.Context, sagaID string) (*domain.TransferSaga, error) {
	s := m.byID[sagaID]
	if s == nil {
		return nil, domain.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *memSaga) GetByBizNo(_ context.Context, tenantID, sourceSystem, bizNo string) (*domain.TransferSaga, error) {
	s := m.byBiz[sagaBizKey(tenantID, sourceSystem, bizNo)]
	if s == nil {
		return nil, domain.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *memSaga) ListOpen(ctx context.Context, tenantID string, limit int) ([]*domain.TransferSaga, error) {
	return m.List(ctx, tenantID, "", limit)
}

func (m *memSaga) List(_ context.Context, tenantID, status string, limit int) ([]*domain.TransferSaga, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []*domain.TransferSaga
	for _, s := range m.byID {
		if tenantID != "" && s.TenantID != tenantID {
			continue
		}
		if status != "" && s.Status != status {
			continue
		}
		if status == "" && (s.Status == domain.SagaCompleted || s.Status == domain.SagaFailed) {
			continue
		}
		cp := *s
		out = append(out, &cp)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func TestSagaCrossShardResume(t *testing.T) {
	ctx := context.Background()
	b, st := setupBooks(t)
	sgStore := newMemSaga()
	b.WithSaga(sgStore)
	b.UsePhase3(nil, nil, nil, nil, func(a, b string) bool { return a == b })
	from := domain.Holder{Type: domain.HolderUser, ID: "u1"}
	to := domain.Holder{Type: domain.HolderUser, ID: "u2"}
	if _, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdCredit, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:saga-in", Holder: from, AssetCode: "POINT", Amount: 40,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := b.Execute(ctx, domain.CommandRequest{
		Command: domain.CmdTransfer, TenantID: "t_default", SourceSystem: "wallet",
		BizNo: "wallet:saga-tf", Holder: from, ToHolder: &to, AssetCode: "POINT", Amount: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Account == nil || res.ToAccount == nil || res.Account.Available != "30" || res.ToAccount.Available != "10" {
		t.Fatalf("saga transfer account=%+v to=%+v", res.Account, res.ToAccount)
	}
	open, err := b.ListSagas(ctx, "t_default", "", 10)
	if err != nil || len(open) != 0 {
		t.Fatalf("open sagas=%d err=%v", len(open), err)
	}
	done, err := b.ListSagas(ctx, "t_default", domain.SagaCompleted, 10)
	if err != nil || len(done) != 1 {
		t.Fatalf("completed sagas=%d err=%v", len(done), err)
	}

	pending := &domain.TransferSaga{
		SagaID: "sg_resume", TenantID: "t_default", SourceSystem: "wallet", BizNo: "wallet:saga-resume",
		FromType: from.Type, FromID: from.ID, ToType: to.Type, ToID: to.ID,
		AssetCode: "POINT", Amount: 8, Status: domain.SagaPending,
		OutBizNo: "wallet:saga-resume:xshard:out", InBizNo: "wallet:saga-resume:xshard:in",
		RollbackNo: "wallet:saga-resume:xshard:rollback",
	}
	if err := sgStore.Create(ctx, pending); err != nil {
		t.Fatal(err)
	}
	got, err := b.ResumeSaga(ctx, pending.SagaID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.SagaCompleted {
		t.Fatalf("resume status=%s err=%s", got.Status, got.LastError)
	}
	fromAcc, _ := memAccount{st}.Get(ctx, "t_default", from, "POINT")
	toAcc, _ := memAccount{st}.Get(ctx, "t_default", to, "POINT")
	if fromAcc.Available != 22 || toAcc.Available != 18 {
		t.Fatalf("after resume from=%d to=%d", fromAcc.Available, toAcc.Available)
	}
	n, err := b.ResumeOpenSagas(ctx, 10)
	if err != nil || n != 0 {
		t.Fatalf("no open sagas n=%d err=%v", n, err)
	}
}
