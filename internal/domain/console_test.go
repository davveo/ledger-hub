package domain

import "testing"

func TestConsoleAllowed(t *testing.T) {
	if !ConsoleAllowed(ConsoleRoleReadonly, "GET", "/api/v1/ledger/console/overview", nil) {
		t.Fatal("readonly GET")
	}
	if ConsoleAllowed(ConsoleRoleReadonly, "POST", "/api/v1/ledger/commands/reverse", nil) {
		t.Fatal("readonly cannot reverse")
	}
	if !ConsoleAllowed(ConsoleRoleCorrection, "POST", "/api/v1/ledger/commands/reverse", nil) {
		t.Fatal("correction reverse")
	}
	if ConsoleAllowed(ConsoleRoleCorrection, "POST", "/api/v1/ledger/ops/jobs/saga", nil) {
		t.Fatal("correction cannot run jobs")
	}
	if !ConsoleAllowed(ConsoleRoleJobs, "POST", "/api/v1/ledger/ops/jobs/saga", nil) {
		t.Fatal("jobs can run saga job")
	}
	if ConsoleAllowed(ConsoleRoleJobs, "POST", "/api/v1/ledger/commands/reverse", nil) {
		t.Fatal("jobs cannot reverse")
	}
	if !ConsoleAllowed(ConsoleRoleCorrection, "POST", "/api/v1/ledger/commands", []byte(`{"command":"Capture"}`)) {
		t.Fatal("correction capture via aggregate")
	}
	if ConsoleAllowed(ConsoleRoleCorrection, "POST", "/api/v1/ledger/commands", []byte(`{"command":"Credit"}`)) {
		t.Fatal("correction cannot credit")
	}
	if !ConsoleAllowed(ConsoleRoleAdmin, "POST", "/api/v1/ledger/assets", nil) {
		t.Fatal("admin assets")
	}
	if !ConsoleAllowed("", "POST", "/api/v1/ledger/assets", nil) {
		t.Fatal("empty role is admin")
	}
}

func TestIsDemoSaga(t *testing.T) {
	if !IsDemoSaga(DemoSagaJSON) {
		t.Fatal("marker")
	}
	if IsDemoSaga("") || IsDemoSaga(`{"ok":1}`) {
		t.Fatal("non demo")
	}
}
