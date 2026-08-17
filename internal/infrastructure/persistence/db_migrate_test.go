package persistence

import "testing"

func TestAssignReconcileJobVersions(t *testing.T) {
	rows := []LedgerReconcileJob{
		{ID: 1, TenantID: "t_default", BizDate: "2026-08-14", JobType: "", Version: 1},
		{ID: 2, TenantID: "t_default", BizDate: "2026-08-14", JobType: "daily", Version: 1},
		{ID: 3, TenantID: "t_default", BizDate: "2026-08-15", JobType: "daily", Version: 1},
	}
	got := assignReconcileJobVersions(rows)
	if got[0].JobType != "daily" || got[0].Version != 1 {
		t.Fatalf("first: %+v", got[0])
	}
	if got[1].Version != 2 {
		t.Fatalf("duplicate should bump version, got %d", got[1].Version)
	}
	if got[2].Version != 1 {
		t.Fatalf("different date should stay version 1, got %d", got[2].Version)
	}
}
