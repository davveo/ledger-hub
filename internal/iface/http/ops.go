package httpserver

import (
	"encoding/csv"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/internal/domain"
)

func (s *Server) operator(c *gin.Context) string {
	if o := c.GetHeader("X-Operator"); o != "" {
		return o
	}
	return "console"
}

func (s *Server) listAccountEntries(c *gin.Context) {
	list, err := s.query.EntriesByAccount(c.Request.Context(), s.tenantID(c, ""), c.Param("account_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, gin.H{"items": list})
}

func (s *Server) listOpenDiffs(c *gin.Context) {
	if s.recon == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	list, err := s.recon.ListOpenDiffs(c.Request.Context(), s.tenantID(c, ""), parsePage(c).Limit)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func (s *Server) listGatewayAudits(c *gin.Context) {
	if s.audit == nil {
		if wantsCSV(c) {
			writeCSV(c, "gateway-audits.csv", []string{"audit_id", "client_id", "tenant_id", "method", "path", "status", "remote_addr", "request_id", "operator", "created_at"}, nil)
			return
		}
		ok(c, []interface{}{})
		return
	}
	q, err := parseAuditQuery(c)
	if err != nil {
		fail(c, err)
		return
	}
	list, err := s.audit.List(c.Request.Context(), q)
	if err != nil {
		fail(c, err)
		return
	}
	if wantsCSV(c) {
		rows := make([][]string, 0, len(list))
		for _, a := range list {
			rows = append(rows, []string{
				a.AuditID, a.ClientID, a.TenantID, a.Method, a.Path,
				strconv.Itoa(a.Status), a.RemoteAddr, a.RequestID, a.Operator, a.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		writeCSV(c, "gateway-audits.csv", []string{"audit_id", "client_id", "tenant_id", "method", "path", "status", "remote_addr", "request_id", "operator", "created_at"}, rows)
		return
	}
	ok(c, list)
}

func (s *Server) listOpsActions(c *gin.Context) {
	if s.jobs == nil {
		if wantsCSV(c) {
			writeCSV(c, "ops-actions.csv", []string{"audit_id", "operator", "action", "tenant_id", "target", "detail", "created_at"}, nil)
			return
		}
		ok(c, []interface{}{})
		return
	}
	q, err := parseAuditQuery(c)
	if err != nil {
		fail(c, err)
		return
	}
	list, err := s.jobs.ListActions(c.Request.Context(), q)
	if err != nil {
		fail(c, err)
		return
	}
	if wantsCSV(c) {
		rows := make([][]string, 0, len(list))
		for _, a := range list {
			rows = append(rows, []string{
				a.AuditID, a.Operator, a.Action, a.TenantID, a.Target, a.Detail, a.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		writeCSV(c, "ops-actions.csv", []string{"audit_id", "operator", "action", "tenant_id", "target", "detail", "created_at"}, rows)
		return
	}
	ok(c, list)
}

func (s *Server) listLimitAlerts(c *gin.Context) {
	if s.limiter == nil {
		ok(c, []interface{}{})
		return
	}
	ok(c, s.limiter.ListAlerts(c.Request.Context(), s.tenantID(c, ""), parsePage(c).Limit))
}

func (s *Server) listOpsJobs(c *gin.Context) {
	if s.jobs == nil {
		ok(c, gin.H{"items": []interface{}{}, "last_success": map[string]interface{}{}})
		return
	}
	list, err := s.jobs.ListRuns(c.Request.Context(), parsePage(c).Limit)
	if err != nil {
		fail(c, err)
		return
	}
	last, err := s.jobs.LastSuccess(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, gin.H{"items": list, "last_success": last})
}

func (s *Server) expirePreview(c *gin.Context) {
	if s.jobs == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	now := time.Now().UTC()
	if asOf := c.Query("as_of"); asOf != "" {
		t, err := time.Parse("2006-01-02", asOf)
		if err != nil {
			fail(c, domain.Keyed(domain.CodeDateNotISO, domain.KeyDateNotISO))
			return
		}
		now = t
	}
	list, err := s.jobs.ExpirePreview(c.Request.Context(), s.tenantID(c, ""), now)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func (s *Server) runOpsJob(c *gin.Context) {
	if s.jobs == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	var body struct {
		Date string `json:"date"`
	}
	_ = c.ShouldBindJSON(&body)
	name := c.Param("name")
	var run *domain.OpsRun
	switch name {
	case "release_expired":
		run = s.jobs.ReleaseExpired(c.Request.Context())
	case "reconcile":
		run = s.jobs.Reconcile(c.Request.Context(), body.Date)
	case "expire":
		run = s.jobs.Expire(c.Request.Context())
	case "fx_feed":
		run = s.jobs.FxFeed(c.Request.Context())
	case "idempotency":
		run = s.jobs.PurgeIdempotency(c.Request.Context())
	case "saga":
		run = s.jobs.ResumeSagas(c.Request.Context())
	case "queued-reconcile":
		run = s.jobs.DrainReconcile(c.Request.Context())
	default:
		fail(c, domain.Keyed(domain.CodeUnknownJob, domain.KeyUnknownJob))
		return
	}
	s.jobs.Record(c.Request.Context(), s.operator(c), "job:"+name, s.tenantID(c, ""), run.RunID, run.Detail)
	ok(c, run)
}

func (s *Server) listSagas(c *gin.Context) {
	if s.books == nil {
		ok(c, []interface{}{})
		return
	}
	list, err := s.books.ListSagas(c.Request.Context(), s.tenantID(c, ""), c.Query("status"), parsePage(c).Limit)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func (s *Server) retrySaga(c *gin.Context) {
	if s.books == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	sg, err := s.books.ResumeSaga(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	if s.jobs != nil {
		s.jobs.Record(c.Request.Context(), s.operator(c), "saga_retry", s.tenantID(c, ""), c.Param("id"), sg.Status)
	}
	ok(c, sg)
}

func (s *Server) compensateSaga(c *gin.Context) {
	if s.books == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	sg, err := s.books.CompensateSaga(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	if s.jobs != nil {
		s.jobs.Record(c.Request.Context(), s.operator(c), "saga_compensate", s.tenantID(c, ""), c.Param("id"), sg.Status)
	}
	ok(c, sg)
}

func (s *Server) assignDiff(c *gin.Context) {
	if s.recon == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	var body struct {
		Assignee string `json:"assignee"`
		Note     string `json:"note"`
	}
	_ = c.ShouldBindJSON(&body)
	d, err := s.recon.AssignDiff(c.Request.Context(), s.tenantID(c, ""), c.Param("id"), body.Assignee, body.Note, s.operator(c))
	if err != nil {
		fail(c, err)
		return
	}
	if s.jobs != nil {
		s.jobs.Record(c.Request.Context(), s.operator(c), "assign_diff", s.tenantID(c, ""), c.Param("id"), body.Assignee)
	}
	ok(c, d)
}

func (s *Server) listDiffEvents(c *gin.Context) {
	if s.recon == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	list, err := s.recon.ListDiffEvents(c.Request.Context(), s.tenantID(c, ""), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func (s *Server) rerunReconcile(c *gin.Context) {
	if s.recon == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	job, err := s.recon.Rerun(c.Request.Context(), s.tenantID(c, ""), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(202, envelope{Code: 0, Data: gin.H{"job_id": job.JobID, "status": job.Status, "version": job.Version}})
}

func (s *Server) listConfigRevisions(c *gin.Context) {
	if s.revs == nil {
		ok(c, []interface{}{})
		return
	}
	list, err := s.revs.List(c.Request.Context(), parsePage(c).Limit)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func wantsCSV(c *gin.Context) bool {
	return strings.EqualFold(c.Query("format"), "csv")
}

func parseAuditQuery(c *gin.Context) (domain.AuditQuery, error) {
	from, to, err := parseAuditTimeRange(c)
	if err != nil {
		return domain.AuditQuery{}, err
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	return domain.AuditQuery{
		TenantID: c.Query("tenant_id"),
		Operator: c.Query("operator"),
		ClientID: c.Query("client_id"),
		From:     from,
		To:       to,
		Limit:    limit,
	}.Clamp(50, 500), nil
}

func parseAuditTimeRange(c *gin.Context) (*time.Time, *time.Time, error) {
	from, err := parseAuditTime(c.Query("from"), false)
	if err != nil {
		return nil, nil, domain.Keyed(domain.CodeTimeFromInvalid, domain.KeyTimeFromInvalid)
	}
	to, err := parseAuditTime(c.Query("to"), true)
	if err != nil {
		return nil, nil, domain.Keyed(domain.CodeTimeToInvalid, domain.KeyTimeToInvalid)
	}
	if from != nil && to != nil && !from.Before(*to) {
		return nil, nil, domain.Keyed(domain.CodeTimeOrderInvalid, domain.KeyTimeOrderInvalid)
	}
	return from, to, nil
}

func parseAuditTime(raw string, endOfDate bool) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, err
	}
	if endOfDate {
		t = t.Add(24 * time.Hour)
	}
	return &t, nil
}

func writeCSV(c *gin.Context, filename string, header []string, rows [][]string) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Status(200)
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(c.Writer)
	_ = w.Write(header)
	for _, row := range rows {
		_ = w.Write(row)
	}
	w.Flush()
}
