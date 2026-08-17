package application

import (
	"context"
	"testing"

	"github.com/davveo/ledger-hub/internal/domain"
)

func TestReconcileEnqueueUniquenessAndForce(t *testing.T) {
	store := NewMemoryReconcile()
	s := NewReconcileService(nil, nil, nil, store)
	ctx := context.Background()

	j1, err := s.Enqueue(ctx, "t_default", "2026-08-16", "order", "POINT", "", false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if j1.Version != 1 || j1.Status != domain.ReconJobQueued {
		t.Fatalf("first job %+v", j1)
	}
	j2, err := s.Enqueue(ctx, "t_default", "2026-08-16", "order", "POINT", "", false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if j2.JobID != j1.JobID {
		t.Fatalf("want reuse %s got %s", j1.JobID, j2.JobID)
	}
	j3, err := s.Enqueue(ctx, "t_default", "2026-08-16", "order", "POINT", "", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if j3.JobID == j1.JobID || j3.Version != 2 {
		t.Fatalf("force new version %+v", j3)
	}
}

func TestDiffAssignResolve(t *testing.T) {
	store := NewMemoryReconcile()
	s := NewReconcileService(nil, nil, nil, store)
	ctx := context.Background()
	job := &domain.ReconcileJob{JobID: "rj_1", TenantID: "t_default", Date: "2026-08-16", Status: domain.ReconJobDone, JobType: "daily", Version: 1}
	_ = store.CreateJob(ctx, job)
	diff := &domain.ReconcileDiff{DiffID: "rd_1", JobID: "rj_1", Status: domain.DiffStatusOpen}
	_ = store.CreateDiffs(ctx, []*domain.ReconcileDiff{diff})

	got, err := s.AssignDiff(ctx, "t_default", "rd_1", "ops_zhang", "look", "console")
	if err != nil {
		t.Fatal(err)
	}
	if got.Assignee != "ops_zhang" {
		t.Fatalf("assignee %s", got.Assignee)
	}
	if err := s.ResolveDiff(ctx, "t_default", "rd_1", "done", "ops_zhang"); err != nil {
		t.Fatal(err)
	}
	d, err := store.GetDiff(ctx, "rd_1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != domain.DiffStatusResolved || d.ClosedBy != "ops_zhang" || d.ClosedAt == nil {
		t.Fatalf("resolve fields %+v", d)
	}
	ev, err := s.ListDiffEvents(ctx, "t_default", "rd_1")
	if err != nil || len(ev) < 2 {
		t.Fatalf("events %v %v", ev, err)
	}
	if _, err := s.AssignDiff(ctx, "t_other", "rd_1", "x", "", "x"); err == nil {
		t.Fatal("cross tenant should 404")
	}
}
