package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/domain"
)

type Server struct {
	assets   *application.AssetService
	accounts *application.AccountService
	books    *application.Bookkeeping
	query    *application.QueryService
	recon    *application.ReconcileService
	tenant   string
}

func New(assets *application.AssetService, accounts *application.AccountService, books *application.Bookkeeping, query *application.QueryService, recon *application.ReconcileService, defaultTenant string) *Server {
	return &Server{assets: assets, accounts: accounts, books: books, query: query, recon: recon, tenant: defaultTenant}
}

func (s *Server) Engine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), requestID(), accessLog())
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "ledger-api"})
	})
	g := r.Group("/api/v1/ledger")
	{
		g.POST("/assets", s.registerAsset)
		g.GET("/assets/:asset_code", s.getAsset)

		g.POST("/accounts/open", s.openAccount)
		g.GET("/accounts", s.getAccount)
		g.GET("/accounts/:account_id", s.getAccountByID)

		g.POST("/commands", s.dispatchCommand)
		g.POST("/commands/credit", s.wrap(domain.CmdCredit))
		g.POST("/commands/debit", s.wrap(domain.CmdDebit))
		g.POST("/commands/freeze", s.wrap(domain.CmdFreeze))
		g.POST("/commands/capture", s.wrap(domain.CmdCapture))
		g.POST("/commands/release", s.wrap(domain.CmdRelease))
		g.POST("/commands/transfer", s.wrap(domain.CmdTransfer))
		g.POST("/commands/exchange", s.wrap(domain.CmdExchange))

		g.GET("/entries", s.listEntries)
		g.GET("/freezes/:freeze_id", s.getFreeze)
		g.GET("/freezes", s.getFreezeByBizNo)

		g.POST("/reconcile/jobs", s.triggerReconcile)
		g.GET("/reconcile/jobs/:id", s.getReconcileJob)
		g.GET("/reconcile/reports/:date", s.getReconcileReport)
		g.POST("/reconcile/diffs/:id/resolve", s.resolveDiff)
	}
	return r
}

type envelope struct {
	Code    int         `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, envelope{Code: 0, Data: data})
}

func fail(c *gin.Context, err error) {
	if de, okErr := err.(*domain.Error); okErr {
		status := http.StatusBadRequest
		switch de.Code {
		case domain.CodeNotFound:
			status = http.StatusNotFound
		case domain.CodeForbidden:
			status = http.StatusForbidden
		case domain.CodeIdempotencyConflict:
			status = http.StatusConflict
		case domain.CodeInsufficient, domain.CodeFreezeStateInvalid:
			status = http.StatusUnprocessableEntity
		case domain.CodeRateLimited:
			status = http.StatusTooManyRequests
		case domain.CodeNotImplemented:
			status = http.StatusNotImplemented
		case domain.CodeInternal:
			status = http.StatusInternalServerError
		}
		c.JSON(status, envelope{Code: de.Code, Message: de.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, envelope{Code: domain.CodeInternal, Message: err.Error()})
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
	ExpireAt     string                 `json:"expire_at"`
	Ext          map[string]interface{} `json:"ext"`
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
		fail(c, domain.NewError(domain.CodeInvalidParam, err.Error()))
		return
	}
	cmd := forced
	if cmd == "" {
		cmd = domain.Command(dto.Command)
	}
	amount, err := parseAmount(dto.Amount)
	if err != nil && dto.Amount != "" {
		fail(c, domain.ErrInvalidParam)
		return
	}
	if dto.From != nil && dto.From.Amount != "" {
		amount, _ = parseAmount(dto.From.Amount)
		if dto.AssetCode == "" {
			dto.AssetCode = dto.From.AssetCode
		}
	}
	var toAmount int64
	toAsset := ""
	if dto.To != nil {
		toAmount, _ = parseAmount(dto.To.Amount)
		toAsset = dto.To.AssetCode
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
		Ext:          dto.Ext,
	}
	if dto.ExpireAt != "" {
		t, err := time.Parse(time.RFC3339, dto.ExpireAt)
		if err != nil {
			fail(c, domain.NewError(domain.CodeInvalidParam, "expire_at 需为 RFC3339"))
			return
		}
		req.ExpireAt = &t
	}
	res, err := s.books.Execute(c.Request.Context(), req)
	if err != nil {
		fail(c, err)
		return
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
		fail(c, domain.NewError(domain.CodeInvalidParam, err.Error()))
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
		fail(c, domain.NewError(domain.CodeInvalidParam, err.Error()))
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
	holder := domain.Holder{
		Type: domain.HolderType(c.Query("holder_type")),
		ID:   c.Query("holder_id"),
	}
	acc, err := s.accounts.Get(c.Request.Context(), s.tenantID(c, ""), holder, c.Query("asset_code"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, accountView(acc))
}

func (s *Server) getAccountByID(c *gin.Context) {
	acc, err := s.accounts.GetByID(c.Request.Context(), c.Param("account_id"))
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
	var from, to *time.Time
	if v := c.Query("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err == nil {
			from = &t
		}
	}
	if v := c.Query("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err == nil {
			to = &t
		}
	}
	list, err := s.query.EntriesByHolder(c.Request.Context(), tenant, holder, c.Query("asset_code"), from, to)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

func (s *Server) getFreeze(c *gin.Context) {
	fz, err := s.query.FreezeByID(c.Request.Context(), c.Param("freeze_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, fz)
}

func (s *Server) getFreezeByBizNo(c *gin.Context) {
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
		BizLines     []struct {
			BizNo     string `json:"biz_no"`
			Command   string `json:"command"`
			AssetCode string `json:"asset_code"`
			Amount    string `json:"amount"`
		} `json:"biz_lines"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, domain.NewError(domain.CodeInvalidParam, err.Error()))
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
	rep, err := s.recon.Trigger(c.Request.Context(), s.tenantID(c, ""), body.Date, body.SourceSystem, body.AssetCode, lines)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, rep)
}

func (s *Server) getReconcileJob(c *gin.Context) {
	rep, err := s.recon.GetJob(c.Request.Context(), c.Param("id"))
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

func (s *Server) resolveDiff(c *gin.Context) {
	var body struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := s.recon.ResolveDiff(c.Request.Context(), c.Param("id"), body.Note); err != nil {
		fail(c, err)
		return
	}
	ok(c, gin.H{"diff_id": c.Param("id"), "status": domain.DiffStatusResolved})
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
	return strconv.ParseInt(s, 10, 64)
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
