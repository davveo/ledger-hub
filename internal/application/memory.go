package application

import (
	"context"
	"sync"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

type MemoryReconcile struct {
	mu     sync.Mutex
	jobs   map[string]*domain.ReconcileJob
	diffs  map[string]*domain.ReconcileDiff
	events map[string][]*domain.ReconcileDiffEvent
}

func NewMemoryReconcile() *MemoryReconcile {
	return &MemoryReconcile{
		jobs:   map[string]*domain.ReconcileJob{},
		diffs:  map[string]*domain.ReconcileDiff{},
		events: map[string][]*domain.ReconcileDiffEvent{},
	}
}

func (m *MemoryReconcile) CreateJob(_ context.Context, job *domain.ReconcileJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *job
	m.jobs[job.JobID] = &cp
	return nil
}

func (m *MemoryReconcile) UpdateJob(_ context.Context, job *domain.ReconcileJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.jobs[job.JobID] == nil {
		return domain.ErrNotFound
	}
	cp := *job
	m.jobs[job.JobID] = &cp
	return nil
}

func (m *MemoryReconcile) GetJob(_ context.Context, jobID string) (*domain.ReconcileJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[jobID]
	if j == nil {
		return nil, domain.ErrNotFound
	}
	cp := *j
	return &cp, nil
}

func (m *MemoryReconcile) ListJobs(_ context.Context, tenantID string, limit int) ([]*domain.ReconcileJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.ReconcileJob
	for _, j := range m.jobs {
		if tenantID != "" && j.TenantID != tenantID {
			continue
		}
		cp := *j
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *MemoryReconcile) ListQueuedJobs(_ context.Context, limit int) ([]*domain.ReconcileJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.ReconcileJob
	for _, j := range m.jobs {
		if j.Status != domain.ReconJobQueued {
			continue
		}
		cp := *j
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *MemoryReconcile) LatestJob(_ context.Context, tenantID, date, sourceSystem, assetCode string) (*domain.ReconcileJob, error) {
	return m.FindJobByKey(context.Background(), tenantID, date, sourceSystem, assetCode, domain.ReconJobTypeDaily)
}

func (m *MemoryReconcile) FindJobByKey(_ context.Context, tenantID, date, sourceSystem, assetCode, jobType string) (*domain.ReconcileJob, error) {
	if jobType == "" {
		jobType = domain.ReconJobTypeDaily
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *domain.ReconcileJob
	for _, j := range m.jobs {
		if j.TenantID != tenantID || j.Date != date || j.SourceSystem != sourceSystem || j.AssetCode != assetCode {
			continue
		}
		jt := j.JobType
		if jt == "" {
			jt = domain.ReconJobTypeDaily
		}
		if jt != jobType {
			continue
		}
		if best == nil || j.Version > best.Version {
			cp := *j
			best = &cp
		}
	}
	if best == nil {
		return nil, domain.ErrNotFound
	}
	return best, nil
}

func (m *MemoryReconcile) CreateDiffs(_ context.Context, diffs []*domain.ReconcileDiff) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range diffs {
		cp := *d
		m.diffs[d.DiffID] = &cp
	}
	return nil
}

func (m *MemoryReconcile) ListDiffs(_ context.Context, jobID string) ([]*domain.ReconcileDiff, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.ReconcileDiff
	for _, d := range m.diffs {
		if d.JobID == jobID {
			cp := *d
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *MemoryReconcile) ListOpenDiffs(_ context.Context, tenantID string, limit int) ([]*domain.ReconcileDiff, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.ReconcileDiff
	for _, d := range m.diffs {
		if d.Status != domain.DiffStatusOpen {
			continue
		}
		job := m.jobs[d.JobID]
		if tenantID != "" && (job == nil || job.TenantID != tenantID) {
			continue
		}
		cp := *d
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *MemoryReconcile) GetDiff(_ context.Context, diffID string) (*domain.ReconcileDiff, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.diffs[diffID]
	if d == nil {
		return nil, domain.ErrNotFound
	}
	cp := *d
	return &cp, nil
}

func (m *MemoryReconcile) UpdateDiff(_ context.Context, d *domain.ReconcileDiff) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.diffs[d.DiffID] == nil {
		return domain.ErrNotFound
	}
	cp := *d
	m.diffs[d.DiffID] = &cp
	return nil
}

func (m *MemoryReconcile) ResolveDiff(_ context.Context, diffID, note, operator string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.diffs[diffID]
	if d == nil {
		return domain.ErrNotFound
	}
	now := time.Now().UTC()
	d.Status = domain.DiffStatusResolved
	d.Note = note
	d.ResolvedBy = operator
	d.ClosedBy = operator
	d.ClosedAt = &now
	return nil
}

func (m *MemoryReconcile) CreateDiffEvent(_ context.Context, ev *domain.ReconcileDiffEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *ev
	m.events[ev.DiffID] = append(m.events[ev.DiffID], &cp)
	return nil
}

func (m *MemoryReconcile) ListDiffEvents(_ context.Context, diffID string) ([]*domain.ReconcileDiffEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*domain.ReconcileDiffEvent(nil), m.events[diffID]...), nil
}

type MemoryConfigRev struct {
	mu   sync.Mutex
	list []*domain.ConfigRevision
}

func NewMemoryConfigRev() *MemoryConfigRev { return &MemoryConfigRev{} }

func (m *MemoryConfigRev) Create(_ context.Context, r *domain.ConfigRevision) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *r
	if cp.Version == 0 {
		cp.Version = int64(len(m.list) + 1)
		r.Version = cp.Version
	}
	if cp.RevisionID == "" {
		cp.RevisionID = "cr_mem"
		r.RevisionID = cp.RevisionID
	}
	m.list = append(m.list, &cp)
	return nil
}

func (m *MemoryConfigRev) List(_ context.Context, limit int) ([]*domain.ConfigRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]*domain.ConfigRevision(nil), m.list...)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (m *MemoryConfigRev) Latest(_ context.Context) (*domain.ConfigRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.list) == 0 {
		return nil, domain.ErrNotFound
	}
	cp := *m.list[len(m.list)-1]
	return &cp, nil
}

type MemoryOpsRun struct {
	mu   sync.Mutex
	list []*domain.OpsRun
}

func NewMemoryOpsRun() *MemoryOpsRun { return &MemoryOpsRun{} }

func (m *MemoryOpsRun) Save(_ context.Context, run *domain.OpsRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *run
	m.list = append(m.list, &cp)
	return nil
}

func (m *MemoryOpsRun) List(_ context.Context, limit int) ([]*domain.OpsRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]*domain.OpsRun(nil), m.list...)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (m *MemoryOpsRun) LastSuccess(_ context.Context) (map[string]time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]time.Time{}
	for _, r := range m.list {
		if r.Status == "done" && r.FinishedAt != nil {
			if t, ok := out[r.Name]; !ok || r.FinishedAt.After(t) {
				out[r.Name] = *r.FinishedAt
			}
		}
	}
	return out, nil
}
