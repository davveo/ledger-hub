package domain

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	ConsoleRoleAdmin      = "admin"
	ConsoleRoleReadonly   = "readonly"
	ConsoleRoleCorrection = "correction"
	ConsoleRoleJobs       = "jobs"

	DemoSagaJSON = `{"demo":true}`
)

type AuditQuery struct {
	TenantID string
	Operator string
	ClientID string
	From     *time.Time
	To       *time.Time
	Limit    int
}

func (q AuditQuery) Clamp(def, max int) AuditQuery {
	if q.Limit <= 0 {
		q.Limit = def
	}
	if q.Limit > max {
		q.Limit = max
	}
	return q
}

func NormalizeConsoleRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case ConsoleRoleReadonly, ConsoleRoleCorrection, ConsoleRoleJobs, ConsoleRoleAdmin:
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return ConsoleRoleAdmin
	}
}

func ConsoleAllowed(role, method, path string, body []byte) bool {
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "GET" || m == "HEAD" || m == "OPTIONS" {
		return true
	}
	role = NormalizeConsoleRole(role)
	if role == ConsoleRoleAdmin {
		return true
	}
	path = strings.TrimSuffix(path, "/")
	switch role {
	case ConsoleRoleCorrection:
		return consoleCorrectionWrite(path, body)
	case ConsoleRoleJobs:
		return consoleJobsWrite(path)
	default:
		return false
	}
}

func consoleCorrectionWrite(path string, body []byte) bool {
	switch path {
	case "/api/v1/ledger/commands/reverse", "/api/v1/ledger/commands/capture", "/api/v1/ledger/commands/release":
		return true
	case "/api/v1/ledger/commands":
		cmd := peekJSONCommand(body)
		return cmd == string(CmdReverse) || cmd == string(CmdCapture) || cmd == string(CmdRelease)
	}
	if strings.HasPrefix(path, "/api/v1/ledger/reconcile/diffs/") {
		return strings.HasSuffix(path, "/resolve") || strings.HasSuffix(path, "/assign")
	}
	return false
}

func consoleJobsWrite(path string) bool {
	if path == "/api/v1/ledger/ops/reload" || path == "/api/v1/ledger/reconcile/jobs" {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/ledger/ops/jobs/") {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/ledger/ops/sagas/") {
		return strings.HasSuffix(path, "/retry") || strings.HasSuffix(path, "/compensate")
	}
	if strings.HasPrefix(path, "/api/v1/ledger/reconcile/jobs/") && strings.HasSuffix(path, "/rerun") {
		return true
	}
	return false
}

func peekJSONCommand(body []byte) string {
	var peek struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(body, &peek)
	return peek.Command
}

func IsDemoSaga(resultJSON string) bool {
	return strings.Contains(resultJSON, `"demo":true`)
}
