package domain

import (
	"fmt"
	"strings"
)

func LangFrom(vals ...string) string {
	for _, raw := range vals {
		if raw == "" {
			continue
		}
		part := strings.Split(raw, ",")[0]
		part = strings.Split(part, ";")[0]
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "en") {
			return "en"
		}
		if strings.HasPrefix(part, "zh") {
			return "zh"
		}
	}
	return "zh"
}

func Localize(lang string, e *Error) string {
	if e == nil {
		return ""
	}
	cat := zhMessages
	if lang == "en" {
		cat = enMessages
	}
	tpl, ok := cat[e.Key]
	if !ok || tpl == "" {
		if e.Message != "" {
			return e.Message
		}
		if lang == "en" {
			if t, ok := zhMessages[e.Key]; ok && t != "" {
				tpl = t
				ok = true
			}
		}
		if !ok || tpl == "" {
			if e.Key != "" {
				return string(e.Key)
			}
			return cat[KeyInternal]
		}
	}
	if len(e.Args) == 0 {
		return tpl
	}
	args := make([]interface{}, len(e.Args))
	for i, a := range e.Args {
		args[i] = a
	}
	return fmt.Sprintf(tpl, args...)
}

var zhMessages = map[Key]string{
	KeyOK:                     "成功",
	KeyInvalidParam:           "参数错误",
	KeyJSONInvalid:            "请求体不是合法 JSON，或缺少必填字段",
	KeyMissingField:           "缺少必填字段",
	KeyUnknownCommand:         "未知命令",
	KeyAmountNotInteger:       "金额必须为最小单位整数",
	KeyAmountFromInvalid:      "from.amount 必须为最小单位整数",
	KeyAmountToInvalid:        "to.amount 必须为最小单位整数",
	KeyAmountFeeInvalid:       "fee.amount 必须为最小单位整数",
	KeyAmountToleranceInvalid: "tolerance 必须为最小单位整数",
	KeyTimeNotRFC3339:         "时间需为 RFC3339",
	KeyTimeFromInvalid:        "from 需为 RFC3339",
	KeyTimeToInvalid:          "to 需为 RFC3339",
	KeyTimeOrderInvalid:       "from 必须早于 to",
	KeyTimeSpanExceeded:       "查询时间跨度不能超过 366 天",
	KeyDateNotISO:             "日期需为 YYYY-MM-DD",
	KeyAssetDisabled:          "资产未启用",
	KeyAssetFreezeUnsupported: "资产不支持冻结",
	KeyHolderTypeNotAllowed:   "持有者类型不被该资产允许",
	KeyAccountDisabled:        "账户已停用",
	KeyCaptureExceedsFreeze:   "Capture 金额不能大于剩余冻结",
	KeyTransferNeedsToHolder:  "Transfer 需要 to_holder",
	KeyTransferCrossCurrency:  "Transfer 禁止跨币种，请使用 Exchange",
	KeyExchangeSameAsset:      "Exchange 需要不同的 to.asset_code",
	KeyExchangeAmountInvalid:  "兑换目标金额无效",
	KeyExchangeNeedsRate:      "Exchange 需要汇率",
	KeyFxRateMissing:          "缺少可用汇率快照",
	KeyFxRateFormat:           "汇率格式错误",
	KeyReverseNeedsRelated:    "Reverse 需要 related_biz_no",
	KeyReverseAlreadyReversed: "不能对冲正流水再冲正，请对原单 related_biz_no",
	KeyUnknownJob:             "未知作业",
	KeySagaAlreadyCompleted:   "已完成的 Saga 不能补偿",
	KeyUnknownSagaStatus:      "未知 Saga 状态",
	KeyUnknownTenant:          "未知租户",
	KeyUnknownEvent:           "未知事件类型",
	KeyUnauthorized:           "鉴权失败",
	KeyReplay:                 "nonce 重复，疑似重放",
	KeyConsoleTokenInQuery:    "console_token 禁止出现在 query",
	KeyUnknownClient:          "未知 client_id",
	KeyMissingSignature:       "缺少签名",
	KeyTimestampInvalid:       "时间戳无效",
	KeyTimestampSkew:          "时间戳超出允许窗口",
	KeyUnknownKeyVersion:      "未知密钥版本",
	KeySignVersionUnsupported: "不支持的签名版本",
	KeyNonceRequired:          "V2 签名需要 nonce",
	KeySignatureMismatch:      "签名校验失败",
	KeyForbidden:              "无权对该资产执行该命令",
	KeySourceSystemMismatch:   "source_system 必须与 client_id 一致",
	KeyTenantHeaderMismatch:   "租户与请求体不一致",
	KeyTenantNotAllowed:       "client 无权访问该租户",
	KeyTenantDisabled:         "租户已停用",
	KeyConsoleRoleDenied:      "当前运营角色无权执行该操作",
	KeyNotFound:               "账户或冻结单不存在",
	KeyIdempotencyConflict:    "幂等冲突但命令参数不一致",
	KeyInsufficient:           "余额不足",
	KeyFreezeStateInvalid:     "冻结单状态不允许 Capture/Release",
	KeySlippage:               "兑换金额超出允许滑点",
	KeyCrossShard:             "跨分片转账请经系统科目，禁止直接 Transfer",
	KeyRateLimited:            "触发限额",
	KeyInternal:               "内部错误",
	KeyOptimisticLock:         "账户乐观锁冲突，请重试",
	KeySagaFailed:             "跨分片转账已失败: %s",
	KeySagaCompensated:        "跨分片转账已补偿: %s",
	KeyFreezeLedgerMismatch:   "冻结余额与冻结单不一致",
	KeyNotImplemented:         "能力尚未交付",
	KeyUpstream:               "上游调用失败",
	KeyNotReady:               "服务未就绪",
}

var enMessages = map[Key]string{
	KeyOK:                     "ok",
	KeyInvalidParam:           "invalid parameter",
	KeyJSONInvalid:            "request body is not valid JSON or a required field is missing",
	KeyMissingField:           "required field is missing",
	KeyUnknownCommand:         "unknown command",
	KeyAmountNotInteger:       "amount must be a min-unit integer",
	KeyAmountFromInvalid:      "from.amount must be a min-unit integer",
	KeyAmountToInvalid:        "to.amount must be a min-unit integer",
	KeyAmountFeeInvalid:       "fee.amount must be a min-unit integer",
	KeyAmountToleranceInvalid: "tolerance must be a min-unit integer",
	KeyTimeNotRFC3339:         "timestamp must be RFC3339",
	KeyTimeFromInvalid:        "from must be RFC3339",
	KeyTimeToInvalid:          "to must be RFC3339",
	KeyTimeOrderInvalid:       "from must be earlier than to",
	KeyTimeSpanExceeded:       "query window cannot exceed 366 days",
	KeyDateNotISO:             "date must be YYYY-MM-DD",
	KeyAssetDisabled:          "asset is disabled",
	KeyAssetFreezeUnsupported: "asset does not support freeze",
	KeyHolderTypeNotAllowed:   "holder type is not allowed for this asset",
	KeyAccountDisabled:        "account is disabled",
	KeyCaptureExceedsFreeze:   "capture amount exceeds remaining freeze",
	KeyTransferNeedsToHolder:  "transfer requires to_holder",
	KeyTransferCrossCurrency:  "transfer cannot change currency; use exchange",
	KeyExchangeSameAsset:      "exchange requires a different to.asset_code",
	KeyExchangeAmountInvalid:  "exchange target amount is invalid",
	KeyExchangeNeedsRate:      "exchange requires an FX rate",
	KeyFxRateMissing:          "no usable FX snapshot",
	KeyFxRateFormat:           "invalid FX rate format",
	KeyReverseNeedsRelated:    "reverse requires related_biz_no",
	KeyReverseAlreadyReversed: "cannot reverse a reverse; use the original related_biz_no",
	KeyUnknownJob:             "unknown job",
	KeySagaAlreadyCompleted:   "completed saga cannot be compensated",
	KeyUnknownSagaStatus:      "unknown saga status",
	KeyUnknownTenant:          "unknown tenant",
	KeyUnknownEvent:           "unknown event type",
	KeyUnauthorized:           "authentication failed",
	KeyReplay:                 "duplicate nonce, possible replay",
	KeyConsoleTokenInQuery:    "console_token must not appear in the query string",
	KeyUnknownClient:          "unknown client_id",
	KeyMissingSignature:       "signature is missing",
	KeyTimestampInvalid:       "timestamp is invalid",
	KeyTimestampSkew:          "timestamp is outside the allowed window",
	KeyUnknownKeyVersion:      "unknown key version",
	KeySignVersionUnsupported: "unsupported signature version",
	KeyNonceRequired:          "v2 signature requires a nonce",
	KeySignatureMismatch:      "signature mismatch",
	KeyForbidden:              "not allowed to run this command on the asset",
	KeySourceSystemMismatch:   "source_system must match client_id",
	KeyTenantHeaderMismatch:   "tenant header does not match request body",
	KeyTenantNotAllowed:       "client is not allowed to access this tenant",
	KeyTenantDisabled:         "tenant is disabled",
	KeyConsoleRoleDenied:      "console role is not allowed to perform this action",
	KeyNotFound:               "account or freeze order not found",
	KeyIdempotencyConflict:    "idempotent replay with different parameters",
	KeyInsufficient:           "insufficient balance",
	KeyFreezeStateInvalid:     "freeze order is not in frozen state",
	KeySlippage:               "exchange amount exceeds allowed slippage",
	KeyCrossShard:             "cross-shard transfer must go through the system subject",
	KeyRateLimited:            "rate limit exceeded",
	KeyInternal:               "internal error",
	KeyOptimisticLock:         "account optimistic lock conflict, retry",
	KeySagaFailed:             "cross-shard transfer failed: %s",
	KeySagaCompensated:        "cross-shard transfer compensated: %s",
	KeyFreezeLedgerMismatch:   "frozen balance does not match freeze order",
	KeyNotImplemented:         "capability is not available",
	KeyUpstream:               "upstream call failed",
	KeyNotReady:               "service is not ready",
}
