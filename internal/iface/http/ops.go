package httpserver

import (
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
		ok(c, []interface{}{})
		return
	}
	list, err := s.audit.List(c.Request.Context(), parsePage(c).Limit)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func (s *Server) listOpsActions(c *gin.Context) {
	if s.jobs == nil {
		ok(c, []interface{}{})
		return
	}
	list, err := s.jobs.ListActions(c.Request.Context(), s.tenantID(c, ""), parsePage(c).Limit)
	if err != nil {
		fail(c, err)
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
		ok(c, []interface{}{})
		return
	}
	list, err := s.jobs.ListRuns(c.Request.Context(), parsePage(c).Limit)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
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
			fail(c, domain.NewError(domain.CodeInvalidParam, "as_of 需为 YYYY-MM-DD"))
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
	default:
		fail(c, domain.NewError(domain.CodeInvalidParam, "未知作业"))
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