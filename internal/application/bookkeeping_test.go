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
func (m *mem) List(_ context.Context, tenant string) ([]*domain.Asset, error) { return nil, nil }

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
	return nil, nil
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
func (m *mem) ListByHolder(_ context.Context, _ string, _ domain.Holder, _ string, _, _ *time.Time) ([]*domain.LedgerEntry, error) {
	return m.entries, nil
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
func (m *mem) ListExpired(_ context.Context, _ time.Time, _ int) ([]*domain.FreezeOrder, error) {
	return nil, nil
}
func (m *mem) ListFrozen(_ context.Context, _, _ string) ([]*domain.FreezeOrder, error) {
	return nil, nil
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
func (r memEntry) ListByHolder(ctx context.Context, t string, h domain.Holder, a string, from, to *time.Time) ([]*domain.LedgerEntry, error) {
	return r.m.ListByHolder(ctx, t, h, a, from, to)
}
func (r memEntry) ListByRange(ctx context.Context, t, s, a string, from, to time.Time) ([]*domain.LedgerEntry, error) {
	return r.m.ListByRange(ctx, t, s, a, from, to)
}
func (r memEntry) ListByAccount(ctx context.Context, id string) ([]*domain.LedgerEntry, error) {
	return r.m.ListByAccount(ctx, id)
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
func (r memFreeze) ListExpired(ctx context.Context, n time.Time, l int) ([]*domain.FreezeOrder, error) {
	return r.m.ListExpired(ctx, n, l)
}
func (r memFreeze) ListFrozen(ctx context.Context, t, a string) ([]*domain.FreezeOrder, error) {
	return r.m.ListFrozen(ctx, t, a)
}
func (r memIdem) Get(ctx context.Context, t, s, b string, c domain.Command) (*domain.IdempotencyRecord, error) {
	return r.m.GetIdem(ctx, t, s, b, c)
}
func (r memIdem) Create(ctx context.Context, rec *domain.IdempotencyRecord) error {
	return r.m.CreateIdem(ctx, rec)
}

func setupBooks(t *testing.T) (*Bookkeeping, *mem) {
	t.Helper()
	st := newMem()
	_ = st.Save(context.Background(), &domain.Asset{
		TenantID: "t_default", AssetCode: "POINT", Name: "积分", Status: domain.AssetActive, FreezeSupported: true,
	})
	acl := NewACL([]domain.ACLRule{
		{SourceSystem: "campaign", Commands: []string{"Credit"}, Assets: []string{"POINT"}},
		{SourceSystem: "order", Commands: []string{"Freeze", "Capture", "Release"}, Assets: []string{"POINT"}},
		{SourceSystem: "wallet", Commands: []string{"Transfer", "Credit"}, Assets: []string{"POINT"}},
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
