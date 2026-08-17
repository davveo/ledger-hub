package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/iface/errresp"
	"github.com/davveo/ledger-hub/internal/observability"
)

type Server struct {
	assets   *application.AssetService
	accounts *application.AccountService
	books    *application.Bookkeeping
	query    *application.QueryService
	recon    *application.ReconcileService
	fx       *application.FxService
	tenants  *application.TenantService
	limits   []domain.LimitRule
	acl      *application.ACL
	limiter  *application.Limiter
	reload   func() error
	tenant   string
	jobs     *application.Jobs
	audit    domain.AuditRepository
	cluster  observability.Pinger
	revs     domain.ConfigRevisionRepository
}

func New(assets *application.AssetService, accounts *application.AccountService, books *application.Bookkeeping, query *application.QueryService, recon *application.ReconcileService, defaultTenant string) *Server {
	return &Server{assets: assets, accounts: accounts, books: books, query: query, recon: recon, tenant: defaultTenant}
}

func (s *Server) WithPhase3(fx *application.FxService, tenants *application.TenantService, limits []domain.LimitRule) *Server {
	s.fx = fx
	s.tenants = tenants
	s.limits = limits
	return s
}

func (s *Server) WithOps(acl *application.ACL, limiter *application.Limiter, reload func() error) *Server {
	s.acl = acl
	s.limiter = limiter
	s.reload = reload
	if limiter != nil {
		s.limits = limiter.Rules()
	}
	return s
}

func (s *Server) WithJobs(jobs *application.Jobs) *Server {
	s.jobs = jobs
	return s
}

func (s *Server) WithAuditLog(a domain.AuditRepository) *Server {
	s.audit = a
	return s
}

func (s *Server) WithCluster(p observability.Pinger) *Server {
	s.cluster = p
	return s
}

func (s *Server) WithRevisions(r domain.ConfigRevisionRepository) *Server {
	s.revs = r
	return s
}

func (s *Server) Engine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), requestID(), observability.EnsureTraceparent(), observability.GinTrace("ledger-api"), observability.HTTPMetrics(), observability.RequestLog(nil), accessLog())
	observability.RegisterProbes(r, "ledger-api", observability.ClusterReady(s.cluster))
	r.GET("/metrics", observability.MetricsHandler())
	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/console") })
	r.GET("/console", s.consolePage)
	g := r.Group("/api/v1/ledger")
	{
		g.POST("/assets", s.registerAsset)
		g.GET("/assets", s.listAssets)
		g.GET("/assets/:asset_code", s.getAsset)
		g.POST("/assets/:asset_code/disable", s.setAssetStatus(domain.AssetDisabled))
		g.POST("/assets/:asset_code/enable", s.setAssetStatus(domain.AssetActive))

		g.POST("/accounts/open", s.openAccount)
		g.GET("/accounts", s.getAccount)
		g.GET("/accounts/:account_id", s.getAccountByID)
		g.GET("/accounts/:account_id/entries", s.listAccountEntries)
		g.POST("/accounts/:account_id/disable", s.setAccountStatus(domain.AccountDisabled))
		g.POST("/accounts/:account_id/enable", s.setAccountStatus(domain.AccountActive))

		g.POST("/commands", s.dispatchCommand)
		g.POST("/commands/credit", s.wrap(domain.CmdCredit))
		g.POST("/commands/debit", s.wrap(domain.CmdDebit))
		g.POST("/commands/freeze", s.wrap(domain.CmdFreeze))
		g.POST("/commands/capture", s.wrap(domain.CmdCapture))
		g.POST("/commands/release", s.wrap(domain.CmdRelease))
		g.POST("/commands/transfer", s.wrap(domain.CmdTransfer))
		g.POST("/commands/exchange", s.wrap(domain.CmdExchange))
		g.POST("/commands/reverse", s.wrap(domain.CmdReverse))

		g.GET("/entries", s.listEntries)
		g.GET("/journals/:id", s.getJournal)
		g.GET("/freezes/:freeze_id", s.getFreeze)
		g.GET("/freezes", s.getFreezeByBizNo)

		g.POST("/fx/rates", s.saveFxRate)
		g.GET("/fx/rates", s.listFxRates)
		g.GET("/fx/rates/:rate_id", s.getFxRate)

		g.POST("/tenants", s.saveTenant)
		g.GET("/tenants", s.listTenants)
		g.GET("/tenants/:id", s.getTenant)
		g.POST("/tenants/:id/disable", s.setTenantStatus("disabled"))
		g.POST("/tenants/:id/enable", s.setTenantStatus("active"))

		g.POST("/ops/reload", s.reloadConfig)
		g.GET("/ops/jobs", s.listOpsJobs)
		g.GET("/ops/jobs/expire/preview", s.expirePreview)
		g.POST("/ops/jobs/:name", s.runOpsJob)
		g.GET("/ops/audits", s.listGatewayAudits)
		g.GET("/ops/actions", s.listOpsActions)
		g.GET("/ops/alerts", s.listLimitAlerts)
		g.GET("/ops/sagas", s.listSagas)
		g.POST("/ops/sagas/:id/retry", s.retrySaga)
		g.POST("/ops/sagas/:id/compensate", s.compensateSaga)
		g.GET("/ops/config/revisions", s.listConfigRevisions)
		g.GET("/openapi.yaml", s.openapiSpec)

		g.POST("/reconcile/jobs", s.triggerReconcile)
		g.GET("/reconcile/jobs", s.listReconcileJobs)
		g.GET("/reconcile/jobs/:id", s.getReconcileJob)
		g.POST("/reconcile/jobs/:id/rerun", s.rerunReconcile)
		g.GET("/reconcile/reports/:date", s.getReconcileReport)
		g.GET("/reconcile/files", s.listReconcileFiles)
		g.GET("/reconcile/files/:name", s.getReconcileFile)
		g.GET("/reconcile/diffs", s.listOpenDiffs)
		g.POST("/reconcile/diffs/:id/resolve", s.resolveDiff)
		g.POST("/reconcile/diffs/:id/assign", s.assignDiff)
		g.GET("/reconcile/diffs/:id/events", s.listDiffEvents)

		g.GET("/console/overview", s.consoleOverview)
	}
	return r
}

type envelope struct {
	Code    int         `json:"code"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, envelope{Code: 0, Data: data})
}

func fail(c *gin.Context, err error) {
	errresp.Write(c, err)
}

type commandDTO struct {
	Command      string                 `json:"command"`
	RequestID    string                 `json:"request_id"`
	TenantID     string                 `json:"tenant_id"`
	SourceSystem string                 `json:"source_system" binding:"required"`
	BizType      string                 `json:"biz_type"`
	BizNo        string                 `json:"biz_no" binding:"required"`
	Holder       domain.Holder          `json:"holder"`
	AssetCode    string                 `json:"asset_code"`
	Amount       string                 `json:"amount"`
	FreezeID     string                 `json:"freeze_id"`
	RelatedBizNo string                 `json:"related_biz_no"`
	ToHolder     *domain.Holder         `json:"to_holder"`
	To           *amountAsset           `json:"to"`
	From         *amountAsset           `json:"from"`
	Fee          *amountAsset           `json:"fee"`
	Fx           *fxDTO                 `json:"fx"`
	Tolerance    string                 `json:"tolerance"`
	ExpireAt     string                 `json:"expire_at"`
	Ext          map[string]interface{} `json:"ext"`
}

type fxDTO struct {
	RateID     string `json:"rate_id"`
	BaseAsset  string `json:"base_asset"`
	QuoteAsset string `json:"quote_asset"`
	Rate       string `json:"rate"`
	RateSource string `json:"rate_source"`
	QuotedAt   string `json:"quoted_at"`
}

type amountAsset struct {
	AssetCode string `json:"asset_code"`
	Amount    string `json:"amount"`
}

func (s *Server) tenantID(c *gin.Context, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if t := c.GetHeader("X-Tenant-Id"); t != "" {
		return t
	}
	return s.tenant
}

func (s *Server) wrap(cmd domain.Command) gin.HandlerFunc {
	return func(c *gin.Context) {
		s.handleCommand(c, cmd)
	}
}

func (s *Server) dispatchCommand(c *gin.Context) {
	s.handleCommand(c, "")
}

func (s *Server) handleCommand(c *gin.Context, forced domain.Command) {
	var dto commandDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		fail(c, domain.Keyed(domain.CodeJSONInvalid, domain.KeyJSONInvalid))
		return
	}
	cmd := forced
	if cmd == "" {
		cmd = domain.Command(dto.Command)
	}
	amount, err := parseAmount(dto.Amount)
	if err != nil {
		fail(c, err)
		return
	}
	if dto.From != nil && dto.From.Amount != "" {
		amount, err = parseAmount(dto.From.Amount)
		if err != nil {
			fail(c, domain.Keyed(domain.CodeAmountFromInvalid, domain.KeyAmountFromInvalid))
			return
		}
		if dto.AssetCode == "" {
			dto.AssetCode = dto.From.AssetCode
		}
	}
	var toAmount int64
	toAsset := ""
	if dto.To != nil {
		var err error
		toAmount, err = parseAmount(dto.To.Amount)
		if err != nil {
			fail(c, domain.Keyed(domain.CodeAmountToInvalid, domain.KeyAmountToInvalid))
			return
		}
		toAsset = dto.To.AssetCode
	}
	var feeAmt int64
	feeAsset := ""
	if dto.Fee != nil {
		var err error
		feeAmt, err = parseAmount(dto.Fee.Amount)
		if err != nil {
			fail(c, domain.Keyed(domain.CodeAmountFeeInvalid, domain.KeyAmountFeeInvalid))
			return
		}
		feeAsset = dto.Fee.AssetCode
	}
	tol, err := parseAmount(dto.Tolerance)
	if err != nil {
		fail(c, domain.Keyed(domain.CodeAmountToleranceInvalid, domain.KeyAmountToleranceInvalid))
		return
	}
	req := domain.CommandRequest{
		Command:      cmd,
		RequestID:    dto.RequestID,
		TenantID:     s.tenantID(c, dto.TenantID),
		SourceSystem: dto.SourceSystem,
		BizType:      dto.BizType,
		BizNo:        dto.BizNo,
		Holder:       dto.Holder,
		AssetCode:    dto.AssetCode,
		Amount:       amount,
		FreezeID:     dto.FreezeID,
		RelatedBizNo: dto.RelatedBizNo,
		ToHolder:     dto.ToHolder,
		ToAssetCode:  toAsset,
		ToAmount:     toAmount,
		FeeAsset:     feeAsset,
		FeeAmount:    feeAmt,
		Tolerance:    tol,
		Ext:          dto.Ext,
	}
	if dto.Fx != nil {
		q := &domain.FxQuote{
			RateID:     dto.Fx.RateID,
			BaseAsset:  dto.Fx.BaseAsset,
			QuoteAsset: dto.Fx.QuoteAsset,
			Rate:       dto.Fx.Rate,
			RateSource: dto.Fx.RateSource,
		}
		if dto.Fx.QuotedAt != "" {
			t, err := time.Parse(time.RFC3339, dto.Fx.QuotedAt)
			if err != nil {
				fail(c, domain.Keyed(domain.CodeTimeNotRFC3339, domain.KeyTimeNotRFC3339))
				return
			}
			q.QuotedAt = t
		}
		req.Fx = q
	}
	if err := s.ensureTenant(c, req.TenantID); err != nil {
		fail(c, err)
		return
	}
	if dto.ExpireAt != "" {
		t, err := time.Parse(time.RFC3339, dto.ExpireAt)
		if err != nil {
			fail(c, domain.Keyed(domain.CodeTimeNotRFC3339, domain.KeyTimeNotRFC3339))
			return
		}
		req.ExpireAt = &t
	}
	res, err := s.books.Execute(c.Request.Context(), req)
	if err != nil {
		result := "error"
		if de, okErr := err.(*domain.Error); okErr {
			result = strconv.Itoa(de.Code)
		}
		observability.ObserveCommand(string(req.Command), result)
		fail(c, err)
		return
	}
	result := "ok"
	if res != nil && res.IdempotentReplay {
		result = "idempotent"
	}
	observability.ObserveCommand(string(req.Command), result)
	c.Set("biz_no", req.BizNo)
	if res != nil {
		c.Set("journal_id", res.JournalID)
	}
	if s.jobs != nil && (req.Command == domain.CmdReverse || req.Command == domain.CmdCapture || req.Command == domain.CmdRelease) {
		s.jobs.Record(c.Request.Context(), s.operator(c), string(req.Command), req.TenantID, req.BizNo, req.RelatedBizNo)
	}
	ok(c, res)
}

type assetDTO struct {
	TenantID         string   `json:"tenant_id"`
	AssetCode        string   `json:"asset_code" binding:"required"`
	Name             string   `json:"name" binding:"required"`
	AssetClass       string   `json:"asset_class"`
	CurrencyCode     string   `json:"currency_code"`
	Precision        int      `json:"precision"`
	HolderTypes      []string `json:"holder_types"`
	FreezeSupported  bool     `json:"freeze_supported"`
	OverdraftAllowed bool     `json:"overdraft_allowed"`
	Status           string   `json:"status"`
	Ext              string   `json:"ext"`
}

func (s *Server) registerAsset(c *gin.Context) {
	var dto assetDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		fail(c, domain.Keyed(domain.CodeJSONInvalid, domain.KeyJSONInvalid))
		return
	}
	a := &domain.Asset{
		TenantID:         s.tenantID(c, dto.TenantID),
		AssetCode:        dto.AssetCode,
		Name:             dto.Name,
		AssetClass:       dto.AssetClass,
		CurrencyCode:     dto.CurrencyCode,
		Precision:        dto.Precision,
		HolderTypes:      dto.HolderTypes,
		FreezeSupported:  dto.FreezeSupported,
		OverdraftAllowed: dto.OverdraftAllowed,
		Status:           domain.AssetStatus(dto.Status),
		Ext:              dto.Ext,
	}
	if a.CurrencyCode == "" {
		a.CurrencyCode = a.AssetCode
	}
	if err := s.ensureTenant(c, a.TenantID); err != nil {
		fail(c, err)
		return
	}
	if err := s.assets.Save(c.Request.Context(), a); err != nil {
		fail(c, err)
		return
	}
	ok(c, a)
}

func (s *Server) getAsset(c *gin.Context) {
	a, err := s.assets.Get(c.Request.Context(), s.tenantID(c, ""), c.Param("asset_code"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, a)
}

type openAccountDTO struct {
	TenantID  string        `json:"tenant_id"`
	Holder    domain.Holder `json:"holder" binding:"required"`
	AssetCode string        `json:"asset_code" binding:"required"`
}

func (s *Server) openAccount(c *gin.Context) {
	var dto openAccountDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		fail(c, domain.Keyed(domain.CodeJSONInvalid, domain.KeyJSONInvalid))
		return
	}
	acc, err := s.accounts.Open(c.Request.Context(), s.tenantID(c, dto.TenantID), dto.Holder, dto.AssetCode)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, accountView(acc))
}

func (s *Server) getAccount(c *gin.Context) {
	if c.Query("holder_id") == "" {
		list, err := s.accounts.List(c.Request.Context(), s.tenantID(c, ""), c.Query("asset_code"))
		if err != nil {
			fail(c, err)
			return
		}
		views := make([]gin.H, 0, len(list))
		for _, acc := range list {
			views = append(views, accountView(acc))
		}
		ok(c, views)
		return
	}
	holder := domain.Holder{
		Type: domain.HolderType(c.Query("holder_type")),
		ID:   c.Query("holder_id"),
	}
	if c.Query("asset_code") != "" {
		acc, err := s.accounts.Get(c.Request.Context(), s.tenantID(c, ""), holder, c.Query("asset_code"))
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, accountView(acc))
		return
	}
	list, err := s.accounts.ListByHolder(c.Request.Context(), s.tenantID(c, ""), holder, "")
	if err != nil {
		fail(c, err)
		return
	}
	views := make([]gin.H, 0, len(list))
	for _, acc := range list {
		views = append(views, accountView(acc))
	}
	ok(c, views)
}

func (s *Server) getAccountByID(c *gin.Context) {
	acc, err := s.accounts.GetByID(c.Request.Context(), s.tenantID(c, ""), c.Param("account_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, accountView(acc))
}

func (s *Server) listEntries(c *gin.Context) {
	tenant := s.tenantID(c, "")
	if bizNo := c.Query("biz_no"); bizNo != "" {
		list, err := s.query.EntriesByBizNo(c.Request.Context(), tenant, bizNo)
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, list)
		return
	}
	holder := domain.Holder{Type: domain.HolderType(c.Query("holder_type")), ID: c.Query("holder_id")}
	from, to, err := parseTimeRange(c)
	if err != nil {
		fail(c, err)
		return
	}
	page := parsePage(c)
	list, err := s.query.EntriesByHolder(c.Request.Context(), tenant, holder, c.Query("asset_code"), from, to, page)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, gin.H{"items": list, "limit": page.Limit, "offset": page.Offset})
}

func (s *Server) getFreeze(c *gin.Context) {
	fz, err := s.query.FreezeByID(c.Request.Context(), s.tenantID(c, ""), c.Param("freeze_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, fz)
}

func (s *Server) getFreezeByBizNo(c *gin.Context) {
	if c.Query("expired") == "1" {
		list, err := s.query.ExpiredFreezes(c.Request.Context(), s.tenantID(c, ""), time.Now().UTC(), parsePage(c).Limit)
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, gin.H{"items": list})
		return
	}
	if holderID := c.Query("holder_id"); holderID != "" {
		holder := domain.Holder{Type: domain.HolderType(c.Query("holder_type")), ID: holderID}
		page := parsePage(c)
		list, err := s.query.FreezesByHolder(c.Request.Context(), s.tenantID(c, ""), holder, c.Query("asset_code"), c.Query("status"), page)
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, gin.H{"items": list, "limit": page.Limit, "offset": page.Offset})
		return
	}
	if c.Query("status") == string(domain.FreezeFrozen) || c.Query("status") == "frozen" {
		list, err := s.query.Frozen(c.Request.Context(), s.tenantID(c, ""), c.Query("asset_code"))
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, gin.H{"items": list})
		return
	}
	fz, err := s.query.FreezeByBizNo(c.Request.Context(), s.tenantID(c, ""), c.Query("biz_no"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, fz)
}

func (s *Server) triggerReconcile(c *gin.Context) {
	var body struct {
		Date         string `json:"date"`
		SourceSystem string `json:"source_system"`
		AssetCode    string `json:"asset_code"`
		JobType      string `json:"job_type"`
		ForceNew     bool   `json:"force_new_version"`
		BizLines     []struct {
			BizNo     string `json:"biz_no"`
			Command   string `json:"command"`
			AssetCode string `json:"asset_code"`
			Amount    string `json:"amount"`
		} `json:"biz_lines"`
		ChannelLines []struct {
			BizNo     string `json:"biz_no"`
			Command   string `json:"command"`
			AssetCode string `json:"asset_code"`
			Amount    string `json:"amount"`
		} `json:"channel_lines"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, domain.Keyed(domain.CodeJSONInvalid, domain.KeyJSONInvalid))
		return
	}
	lines := make([]domain.BizLine, 0, len(body.BizLines))
	for _, l := range body.BizLines {
		amt, err := parseAmount(l.Amount)
		if err != nil {
			fail(c, domain.ErrInvalidParam)
			return
		}
		lines = append(lines, domain.BizLine{
			BizNo:     l.BizNo,
			Command:   domain.Command(l.Command),
			AssetCode: l.AssetCode,
			Amount:    amt,
		})
	}
	channels := make([]domain.BizLine, 0, len(body.ChannelLines))
	for _, l := range body.ChannelLines {
		amt, err := parseAmount(l.Amount)
		if err != nil {
			fail(c, domain.ErrInvalidParam)
			return
		}
		channels = append(channels, domain.BizLine{
			BizNo:     l.BizNo,
			Command:   domain.Command(l.Command),
			AssetCode: l.AssetCode,
			Amount:    amt,
		})
	}
	rep, err := s.recon.Enqueue(c.Request.Context(), s.tenantID(c, ""), body.Date, body.SourceSystem, body.AssetCode, body.JobType, body.ForceNew, lines, channels)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, envelope{Code: 0, Data: gin.H{
		"job_id":  rep.JobID,
		"status":  rep.Status,
		"version": rep.Version,
		"phase":   rep.Phase,
	}})
}

func (s *Server) listReconcileJobs(c *gin.Context) {
	if s.recon == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	list, err := s.recon.ListJobs(c.Request.Context(), s.tenantID(c, ""), parsePage(c).Limit)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func (s *Server) listReconcileFiles(c *gin.Context) {
	if s.recon == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	list, err := s.recon.ListFiles()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func (s *Server) getReconcileJob(c *gin.Context) {
	rep, err := s.recon.GetJob(c.Request.Context(), s.tenantID(c, ""), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, rep)
}

func (s *Server) getReconcileReport(c *gin.Context) {
	rep, err := s.recon.ReportByDate(c.Request.Context(), s.tenantID(c, ""), c.Param("date"), c.Query("source_system"), c.Query("asset_code"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, rep)
}

func (s *Server) getReconcileFile(c *gin.Context) {
	p, err := s.recon.FilePath(c.Param("name"))
	if err != nil {
		fail(c, err)
		return
	}
	if s.jobs != nil {
		s.jobs.Record(c.Request.Context(), s.operator(c), "download_file", s.tenantID(c, ""), c.Param("name"), "")
	}
	c.FileAttachment(p, c.Param("name"))
}

func (s *Server) resolveDiff(c *gin.Context) {
	var body struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&body)
	op := s.operator(c)
	if err := s.recon.ResolveDiff(c.Request.Context(), s.tenantID(c, ""), c.Param("id"), body.Note, op); err != nil {
		fail(c, err)
		return
	}
	if s.jobs != nil {
		s.jobs.Record(c.Request.Context(), op, "resolve_diff", s.tenantID(c, ""), c.Param("id"), body.Note)
	}
	ok(c, gin.H{"diff_id": c.Param("id"), "status": domain.DiffStatusResolved, "resolved_by": op})
}

func (s *Server) ensureTenant(c *gin.Context, tenantID string) error {
	if s.tenants == nil {
		return nil
	}
	return s.tenants.Ensure(c.Request.Context(), tenantID)
}

func (s *Server) listAssets(c *gin.Context) {
	list, err := s.assets.List(c.Request.Context(), s.tenantID(c, ""))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func (s *Server) getJournal(c *gin.Context) {
	j, entries, err := s.query.Journal(c.Request.Context(), s.tenantID(c, ""), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, gin.H{"journal": j, "entries": entries})
}

func (s *Server) saveFxRate(c *gin.Context) {
	if s.fx == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	var dto struct {
		RateID     string `json:"rate_id"`
		TenantID   string `json:"tenant_id"`
		BaseAsset  string `json:"base_asset" binding:"required"`
		QuoteAsset string `json:"quote_asset" binding:"required"`
		Rate       string `json:"rate" binding:"required"`
		RateSource string `json:"rate_source"`
		QuotedAt   string `json:"quoted_at"`
		CreatedBy  string `json:"created_by"`
	}
	if err := c.ShouldBindJSON(&dto); err != nil {
		fail(c, domain.Keyed(domain.CodeJSONInvalid, domain.KeyJSONInvalid))
		return
	}
	r := &domain.FxRate{
		RateID:     dto.RateID,
		TenantID:   s.tenantID(c, dto.TenantID),
		BaseAsset:  dto.BaseAsset,
		QuoteAsset: dto.QuoteAsset,
		Rate:       dto.Rate,
		RateSource: dto.RateSource,
		CreatedBy:  dto.CreatedBy,
	}
	if dto.QuotedAt != "" {
		t, err := time.Parse(time.RFC3339, dto.QuotedAt)
		if err != nil {
			fail(c, domain.Keyed(domain.CodeTimeNotRFC3339, domain.KeyTimeNotRFC3339))
			return
		}
		r.QuotedAt = t
	}
	if err := s.ensureTenant(c, r.TenantID); err != nil {
		fail(c, err)
		return
	}
	if err := s.fx.Save(c.Request.Context(), r); err != nil {
		fail(c, err)
		return
	}
	ok(c, r)
}

func (s *Server) listFxRates(c *gin.Context) {
	if s.fx == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	tenant := s.tenantID(c, "")
	if base, quote := c.Query("base"), c.Query("quote"); base != "" && quote != "" {
		at := time.Now().UTC()
		if v := c.Query("at"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				fail(c, domain.Keyed(domain.CodeTimeNotRFC3339, domain.KeyTimeNotRFC3339))
				return
			}
			at = t
		}
		r, err := s.fx.Quote(c.Request.Context(), tenant, base, quote, at)
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, r)
		return
	}
	list, err := s.fx.List(c.Request.Context(), tenant)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func (s *Server) getFxRate(c *gin.Context) {
	if s.fx == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	r, err := s.fx.Get(c.Request.Context(), s.tenantID(c, ""), c.Param("rate_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, r)
}

func (s *Server) saveTenant(c *gin.Context) {
	if s.tenants == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	var dto domain.Tenant
	if err := c.ShouldBindJSON(&dto); err != nil {
		fail(c, domain.Keyed(domain.CodeJSONInvalid, domain.KeyJSONInvalid))
		return
	}
	if err := s.tenants.Save(c.Request.Context(), &dto); err != nil {
		fail(c, err)
		return
	}
	ok(c, dto)
}

func (s *Server) listTenants(c *gin.Context) {
	if s.tenants == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	list, err := s.tenants.List(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func (s *Server) getTenant(c *gin.Context) {
	if s.tenants == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	t, err := s.tenants.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, t)
}

func (s *Server) consoleOverview(c *gin.Context) {
	ctx := c.Request.Context()
	tenant := s.tenantID(c, "")
	assets, _ := s.assets.List(ctx, tenant)
	accs, _ := s.accounts.List(ctx, tenant, c.Query("asset_code"))
	accViews := make([]gin.H, 0, len(accs))
	for _, acc := range accs {
		accViews = append(accViews, accountView(acc))
	}
	var rates []*domain.FxRate
	if s.fx != nil {
		rates, _ = s.fx.List(ctx, tenant)
	}
	var tenants []*domain.Tenant
	if s.tenants != nil {
		tenants, _ = s.tenants.List(ctx)
	}
	limits := s.limits
	if s.limiter != nil {
		limits = s.limiter.Rules()
	}
	var aclRules []domain.ACLRule
	if s.acl != nil {
		aclRules = s.acl.Rules()
	}
	var alerts []domain.LimitAlert
	if s.limiter != nil {
		alerts = s.limiter.ListAlerts(ctx, tenant, 50)
	}
	ok(c, gin.H{
		"tenant_id": tenant,
		"tenants":   tenants,
		"assets":    assets,
		"accounts":  accViews,
		"fx_rates":  rates,
		"limits":    limits,
		"acl":       aclRules,
		"alerts":    alerts,
	})
}

func (s *Server) setAssetStatus(st domain.AssetStatus) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, err := s.assets.SetStatus(c.Request.Context(), s.tenantID(c, ""), c.Param("asset_code"), st)
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, a)
	}
}

func (s *Server) setAccountStatus(st domain.AccountStatus) gin.HandlerFunc {
	return func(c *gin.Context) {
		acc, err := s.accounts.SetStatus(c.Request.Context(), s.tenantID(c, ""), c.Param("account_id"), st)
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, accountView(acc))
	}
}

func (s *Server) setTenantStatus(status string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.tenants == nil {
			fail(c, domain.ErrNotImplemented)
			return
		}
		t, err := s.tenants.SetStatus(c.Request.Context(), c.Param("id"), status)
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, t)
	}
}

func (s *Server) reloadConfig(c *gin.Context) {
	if s.reload == nil {
		fail(c, domain.ErrNotImplemented)
		return
	}
	if err := s.reload(); err != nil {
		fail(c, domain.ErrInternal)
		return
	}
	if s.limiter != nil {
		s.limits = s.limiter.Rules()
	}
	var rules []domain.ACLRule
	if s.acl != nil {
		rules = s.acl.Rules()
	}
	rev, err := application.SaveConfigRevision(c.Request.Context(), s.revs, s.operator(c), rules, s.limits)
	if err != nil {
		fail(c, err)
		return
	}
	if s.jobs != nil {
		detail := ""
		if rev != nil {
			detail = strconv.FormatInt(rev.Version, 10)
		}
		s.jobs.Record(c.Request.Context(), s.operator(c), "reload", s.tenantID(c, ""), "", detail)
	}
	ok(c, gin.H{"reloaded": true, "limits": s.limits, "acl": rules, "revision": rev})
}

func parsePage(c *gin.Context) domain.Page {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	return domain.Page{Limit: limit, Offset: offset}.Clamp(50, 200)
}

func accountView(acc *domain.Account) gin.H {
	return gin.H{
		"account_id":  acc.AccountID,
		"tenant_id":   acc.TenantID,
		"holder_type": acc.HolderType,
		"holder_id":   acc.HolderID,
		"asset_code":  acc.AssetCode,
		"available":   strconv.FormatInt(acc.Available, 10),
		"frozen":      strconv.FormatInt(acc.Frozen, 10),
		"version":     acc.Version,
		"status":      acc.Status,
	}
}

func parseAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if strings.ContainsAny(s, ".eE+") {
		return 0, domain.Keyed(domain.CodeAmountNotInteger, domain.KeyAmountNotInteger)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, domain.Keyed(domain.CodeAmountNotInteger, domain.KeyAmountNotInteger)
	}
	return n, nil
}

func parseTimeRange(c *gin.Context) (*time.Time, *time.Time, error) {
	var from, to *time.Time
	if v := c.Query("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, nil, domain.Keyed(domain.CodeTimeFromInvalid, domain.KeyTimeFromInvalid)
		}
		from = &t
	}
	if v := c.Query("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, nil, domain.Keyed(domain.CodeTimeToInvalid, domain.KeyTimeToInvalid)
		}
		to = &t
	}
	if from != nil && to != nil {
		if !from.Before(*to) {
			return nil, nil, domain.Keyed(domain.CodeTimeOrderInvalid, domain.KeyTimeOrderInvalid)
		}
		if to.Sub(*from) > 366*24*time.Hour {
			return nil, nil, domain.Keyed(domain.CodeTimeSpanExceeded, domain.KeyTimeSpanExceeded)
		}
	}
	return from, to, nil
}

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			id = strconv.FormatInt(time.Now().UnixNano(), 36)
		}
		c.Writer.Header().Set("X-Request-Id", id)
		c.Next()
	}
}

func accessLog() gin.HandlerFunc {
	return gin.Logger()
}
