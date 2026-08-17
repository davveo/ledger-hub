package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

func (s *Bookkeeping) WithSaga(repo domain.SagaRepository) *Bookkeeping {
	s.sagas = repo
	return s
}

func (s *Bookkeeping) crossShardTransfer(ctx context.Context, req domain.CommandRequest) (*domain.CommandResult, error) {
	if s.sagas == nil {
		return s.crossShardOnce(ctx, req)
	}
	hash := requestHash(req)
	ctxFrom := domain.WithHolder(ctx, req.Holder.ID)
	var replay *domain.CommandResult
	if err := s.tx.WithinTx(ctxFrom, func(ctx context.Context) error {
		r, err := s.checkIdempotency(ctx, req, hash)
		replay = r
		return err
	}); err != nil {
		return nil, err
	}
	if replay != nil {
		return replay, nil
	}
	sg, err := s.sagas.GetByBizNo(ctx, req.TenantID, req.SourceSystem, req.BizNo)
	if err != nil && !domain.Is(err, domain.CodeNotFound) {
		return nil, err
	}
	if sg == nil {
		sg = newSaga(req)
		if err := s.sagas.Create(ctx, sg); err != nil {
			existing, getErr := s.sagas.GetByBizNo(ctx, req.TenantID, req.SourceSystem, req.BizNo)
			if getErr != nil {
				return nil, err
			}
			sg = existing
		}
	}
	return s.resumeSaga(ctx, sg, &req, hash)
}

func newSaga(req domain.CommandRequest) *domain.TransferSaga {
	now := time.Now().UTC()
	return &domain.TransferSaga{
		SagaID:       idgen.New("sg_"),
		TenantID:     req.TenantID,
		SourceSystem: req.SourceSystem,
		BizNo:        req.BizNo,
		FromType:     req.Holder.Type,
		FromID:       req.Holder.ID,
		ToType:       req.ToHolder.Type,
		ToID:         req.ToHolder.ID,
		AssetCode:    req.AssetCode,
		Amount:       req.Amount,
		Status:       domain.SagaPending,
		OutBizNo:     req.BizNo + ":xshard:out",
		InBizNo:      req.BizNo + ":xshard:in",
		RollbackNo:   req.BizNo + ":xshard:rollback",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func (s *Bookkeeping) ResumeSaga(ctx context.Context, sagaID string) (*domain.TransferSaga, error) {
	if s.sagas == nil {
		return nil, domain.ErrNotImplemented
	}
	sg, err := s.sagas.Get(ctx, sagaID)
	if err != nil {
		return nil, err
	}
	req := sagaRequest(sg)
	hash := requestHash(req)
	_, err = s.resumeSaga(ctx, sg, &req, hash)
	return sg, err
}

func (s *Bookkeeping) CompensateSaga(ctx context.Context, sagaID string) (*domain.TransferSaga, error) {
	if s.sagas == nil {
		return nil, domain.ErrNotImplemented
	}
	sg, err := s.sagas.Get(ctx, sagaID)
	if err != nil {
		return nil, err
	}
	if sg.Status == domain.SagaCompleted {
		return sg, domain.NewError(domain.CodeInvalidParam, "已完成的 Saga 不能补偿")
	}
	if sg.Status != domain.SagaFailed {
		sg.Status = domain.SagaCompensating
		_ = s.sagas.Update(ctx, sg)
	}
	req := sagaRequest(sg)
	hash := requestHash(req)
	_, err = s.resumeSaga(ctx, sg, &req, hash)
	return sg, err
}

func (s *Bookkeeping) ListSagas(ctx context.Context, tenantID, status string, limit int) ([]*domain.TransferSaga, error) {
	if s.sagas == nil {
		return []*domain.TransferSaga{}, nil
	}
	if status == "open" || status == "" {
		return s.sagas.ListOpen(ctx, tenantID, limit)
	}
	return s.sagas.List(ctx, tenantID, status, limit)
}

func (s *Bookkeeping) ResumeOpenSagas(ctx context.Context, limit int) (int, error) {
	if s.sagas == nil {
		return 0, nil
	}
	list, err := s.sagas.ListOpen(ctx, "", limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, sg := range list {
		req := sagaRequest(sg)
		hash := requestHash(req)
		if _, err := s.resumeSaga(ctx, sg, &req, hash); err == nil {
			n++
		}
	}
	return n, nil
}

func sagaRequest(sg *domain.TransferSaga) domain.CommandRequest {
	to := domain.Holder{Type: sg.ToType, ID: sg.ToID}
	return domain.CommandRequest{
		Command:      domain.CmdTransfer,
		TenantID:     sg.TenantID,
		SourceSystem: sg.SourceSystem,
		BizType:      "xshard",
		BizNo:        sg.BizNo,
		Holder:       domain.Holder{Type: sg.FromType, ID: sg.FromID},
		ToHolder:     &to,
		AssetCode:    sg.AssetCode,
		Amount:       sg.Amount,
	}
}

func (s *Bookkeeping) resumeSaga(ctx context.Context, sg *domain.TransferSaga, req *domain.CommandRequest, hash string) (*domain.CommandResult, error) {
	sg.RetryCount++
	switch sg.Status {
	case domain.SagaCompleted:
		if sg.ResultJSON != "" {
			var res domain.CommandResult
			if json.Unmarshal([]byte(sg.ResultJSON), &res) == nil {
				res.IdempotentReplay = true
				return &res, nil
			}
		}
		return s.crossShardOnce(ctx, *req)
	case domain.SagaFailed:
		return nil, domain.NewError(domain.CodeInternal, "跨分片转账已失败: "+sg.LastError)
	case domain.SagaPending:
		res, err := s.doSagaOut(ctx, sg, *req)
		if err != nil {
			sg.LastError = err.Error()
			_ = s.sagas.Update(ctx, sg)
			return nil, err
		}
		_ = res
		fallthrough
	case domain.SagaOutDone:
		res, err := s.doSagaIn(ctx, sg, *req)
		if err != nil {
			sg.Status = domain.SagaCompensating
			sg.LastError = err.Error()
			_ = s.sagas.Update(ctx, sg)
			_, _ = s.doSagaCompensate(ctx, sg, *req)
			return nil, err
		}
		return s.finishSaga(ctx, sg, *req, hash, res)
	case domain.SagaInDone:
		var res domain.CommandResult
		if sg.ResultJSON != "" {
			_ = json.Unmarshal([]byte(sg.ResultJSON), &res)
		}
		return s.finishSaga(ctx, sg, *req, hash, &res)
	case domain.SagaCompensating:
		_, err := s.doSagaCompensate(ctx, sg, *req)
		if err != nil {
			sg.LastError = err.Error()
			_ = s.sagas.Update(ctx, sg)
			return nil, err
		}
		return nil, domain.NewError(domain.CodeInternal, "跨分片转账已补偿: "+sg.LastError)
	default:
		return nil, domain.NewError(domain.CodeInternal, "未知 Saga 状态")
	}
}

func (s *Bookkeeping) doSagaOut(ctx context.Context, sg *domain.TransferSaga, req domain.CommandRequest) (*domain.CommandResult, error) {
	pending := domain.Holder{Type: domain.HolderSystemSubject, ID: domain.SystemPendingSettlement}
	ctxFrom := domain.WithHolder(ctx, req.Holder.ID)
	outReq := req
	outReq.ToHolder = &pending
	outReq.BizNo = sg.OutBizNo
	outReq.RelatedBizNo = req.BizNo
	res, err := s.transferSameShard(ctxFrom, outReq)
	if err != nil {
		return nil, err
	}
	sg.Status = domain.SagaOutDone
	return res, s.sagas.Update(ctx, sg)
}

func (s *Bookkeeping) doSagaIn(ctx context.Context, sg *domain.TransferSaga, req domain.CommandRequest) (*domain.CommandResult, error) {
	pending := domain.Holder{Type: domain.HolderSystemSubject, ID: domain.SystemPendingSettlement}
	ctxTo := domain.WithHolder(ctx, req.ToHolder.ID)
	inReq := req
	inReq.Holder = pending
	inReq.BizNo = sg.InBizNo
	inReq.RelatedBizNo = req.BizNo
	res, err := s.transferSameShard(ctxTo, inReq)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(res)
	sg.ResultJSON = string(raw)
	sg.Status = domain.SagaInDone
	return res, s.sagas.Update(ctx, sg)
}

func (s *Bookkeeping) doSagaCompensate(ctx context.Context, sg *domain.TransferSaga, req domain.CommandRequest) (*domain.CommandResult, error) {
	ctxFrom := domain.WithHolder(ctx, req.Holder.ID)
	res, err := s.reverse(ctxFrom, domain.CommandRequest{
		Command:      domain.CmdReverse,
		TenantID:     req.TenantID,
		SourceSystem: req.SourceSystem,
		BizType:      "xshard_rollback",
		BizNo:        sg.RollbackNo,
		RelatedBizNo: sg.OutBizNo,
		Holder:       req.Holder,
		AssetCode:    req.AssetCode,
	})
	if err != nil && !domain.Is(err, domain.CodeNotFound) && !domain.Is(err, domain.CodeIdempotencyConflict) {
		return nil, err
	}
	sg.Status = domain.SagaFailed
	_ = s.sagas.Update(ctx, sg)
	return res, nil
}

func (s *Bookkeeping) finishSaga(ctx context.Context, sg *domain.TransferSaga, req domain.CommandRequest, hash string, inRes *domain.CommandResult) (*domain.CommandResult, error) {
	res := s.crossShardView(ctx, req, inRes)
	raw, _ := json.Marshal(res)
	sg.ResultJSON = string(raw)
	ctxFrom := domain.WithHolder(ctx, req.Holder.ID)
	if err := s.tx.WithinTx(ctxFrom, func(ctx context.Context) error {
		return s.saveIdempotency(ctx, req, hash, res)
	}); err != nil {
		sg.LastError = err.Error()
		_ = s.sagas.Update(ctx, sg)
		return res, err
	}
	sg.Status = domain.SagaCompleted
	sg.LastError = ""
	if err := s.sagas.Update(ctx, sg); err != nil {
		return res, err
	}
	return res, nil
}

func (s *Bookkeeping) crossShardView(ctx context.Context, req domain.CommandRequest, inRes *domain.CommandResult) *domain.CommandResult {
	res := &domain.CommandResult{Accepted: true}
	if inRes != nil {
		res.JournalID = inRes.JournalID
		res.EntryIDs = append([]string{}, inRes.EntryIDs...)
		res.ToAccount = inRes.ToAccount
	}
	if acc, err := s.accs.Get(ctx, req.TenantID, req.Holder, req.AssetCode); err == nil {
		res.Account = toBalance(acc)
	}
	if req.ToHolder != nil {
		if acc, err := s.accs.Get(ctx, req.TenantID, *req.ToHolder, req.AssetCode); err == nil {
			res.ToAccount = toBalance(acc)
		}
	}
	return res
}

func (s *Bookkeeping) crossShardOnce(ctx context.Context, req domain.CommandRequest) (*domain.CommandResult, error) {
	hash := requestHash(req)
	ctxFrom := domain.WithHolder(ctx, req.Holder.ID)
	var replay *domain.CommandResult
	if err := s.tx.WithinTx(ctxFrom, func(ctx context.Context) error {
		r, err := s.checkIdempotency(ctx, req, hash)
		replay = r
		return err
	}); err != nil {
		return nil, err
	}
	if replay != nil {
		return replay, nil
	}
	pending := domain.Holder{Type: domain.HolderSystemSubject, ID: domain.SystemPendingSettlement}
	outReq := req
	outReq.ToHolder = &pending
	outReq.BizNo = req.BizNo + ":xshard:out"
	outReq.RelatedBizNo = req.BizNo
	res1, err := s.transferSameShard(ctxFrom, outReq)
	if err != nil {
		return nil, err
	}
	ctxTo := domain.WithHolder(ctx, req.ToHolder.ID)
	inReq := req
	inReq.Holder = pending
	inReq.BizNo = req.BizNo + ":xshard:in"
	inReq.RelatedBizNo = req.BizNo
	res2, err := s.transferSameShard(ctxTo, inReq)
	if err != nil {
		_, _ = s.reverse(ctxFrom, domain.CommandRequest{
			Command:      domain.CmdReverse,
			TenantID:     req.TenantID,
			SourceSystem: req.SourceSystem,
			BizType:      "xshard_rollback",
			BizNo:        req.BizNo + ":xshard:rollback",
			RelatedBizNo: outReq.BizNo,
			Holder:       req.Holder,
			AssetCode:    req.AssetCode,
		})
		return nil, err
	}
	res := &domain.CommandResult{
		Accepted:  true,
		JournalID: res2.JournalID,
		EntryIDs:  append(append([]string{}, res1.EntryIDs...), res2.EntryIDs...),
		Account:   res1.Account,
		ToAccount: res2.ToAccount,
	}
	if err := s.tx.WithinTx(ctxFrom, func(ctx context.Context) error {
		return s.saveIdempotency(ctx, req, hash, res)
	}); err != nil {
		return res, err
	}
	return res, nil
}
