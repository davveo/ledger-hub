package domain

import "fmt"

const (
	CodeOK = 0

	CodeInvalidParam            = 40001
	CodeJSONInvalid             = 40002
	CodeMissingField            = 40003
	CodeUnknownCommand          = 40004
	CodeAmountNotInteger        = 40010
	CodeAmountFromInvalid       = 40011
	CodeAmountToInvalid         = 40012
	CodeAmountFeeInvalid        = 40013
	CodeAmountToleranceInvalid  = 40014
	CodeTimeNotRFC3339          = 40020
	CodeTimeFromInvalid         = 40021
	CodeTimeToInvalid           = 40022
	CodeTimeOrderInvalid        = 40023
	CodeTimeSpanExceeded        = 40024
	CodeDateNotISO              = 40025
	CodeAssetDisabled           = 40031
	CodeAssetFreezeUnsupported  = 40032
	CodeHolderTypeNotAllowed    = 40033
	CodeAccountDisabled         = 40034
	CodeCaptureExceedsFreeze    = 40036
	CodeTransferNeedsToHolder   = 40037
	CodeTransferCrossCurrency   = 40038
	CodeExchangeSameAsset       = 40039
	CodeExchangeAmountInvalid   = 40040
	CodeExchangeNeedsRate       = 40041
	CodeFxRateMissing           = 40042
	CodeFxRateFormat            = 40043
	CodeReverseNeedsRelated     = 40044
	CodeReverseAlreadyReversed  = 40045
	CodeUnknownJob              = 40046
	CodeSagaAlreadyCompleted    = 40047
	CodeUnknownSagaStatus       = 40048
	CodeUnknownTenant           = 40049
	CodeUnknownEvent            = 40050

	CodeUnauthorized            = 40100
	CodeReplay                  = 40102
	CodeConsoleTokenInQuery     = 40110
	CodeUnknownClient           = 40111
	CodeMissingSignature        = 40112
	CodeTimestampInvalid        = 40113
	CodeTimestampSkew           = 40114
	CodeUnknownKeyVersion       = 40115
	CodeSignVersionUnsupported  = 40116
	CodeNonceRequired           = 40117
	CodeSignatureMismatch       = 40118

	CodeForbidden               = 40301
	CodeSourceSystemMismatch    = 40310
	CodeTenantHeaderMismatch    = 40311
	CodeTenantNotAllowed        = 40312
	CodeTenantDisabled          = 40313
	CodeConsoleRoleDenied       = 40314

	CodeNotFound                = 40401
	CodeIdempotencyConflict     = 40901
	CodeInsufficient            = 42201
	CodeFreezeStateInvalid      = 42202
	CodeSlippage                = 42203
	CodeCrossShard              = 42204
	CodeRateLimited             = 42901

	CodeInternal                = 50000
	CodeOptimisticLock          = 50010
	CodeSagaFailed              = 50011
	CodeSagaCompensated         = 50012
	CodeFreezeLedgerMismatch    = 50013
	CodeNotImplemented          = 50101
	CodeUpstream                = 50200
	CodeNotReady                = 50300
)

type Key string

const (
	KeyOK                      Key = "OK"
	KeyInvalidParam            Key = "INVALID_PARAM"
	KeyJSONInvalid             Key = "JSON_INVALID"
	KeyMissingField            Key = "MISSING_FIELD"
	KeyUnknownCommand          Key = "UNKNOWN_COMMAND"
	KeyAmountNotInteger        Key = "AMOUNT_NOT_INTEGER"
	KeyAmountFromInvalid       Key = "AMOUNT_FROM_INVALID"
	KeyAmountToInvalid         Key = "AMOUNT_TO_INVALID"
	KeyAmountFeeInvalid        Key = "AMOUNT_FEE_INVALID"
	KeyAmountToleranceInvalid  Key = "AMOUNT_TOLERANCE_INVALID"
	KeyTimeNotRFC3339          Key = "TIME_NOT_RFC3339"
	KeyTimeFromInvalid         Key = "TIME_FROM_INVALID"
	KeyTimeToInvalid           Key = "TIME_TO_INVALID"
	KeyTimeOrderInvalid        Key = "TIME_ORDER_INVALID"
	KeyTimeSpanExceeded        Key = "TIME_SPAN_EXCEEDED"
	KeyDateNotISO              Key = "DATE_NOT_ISO"
	KeyAssetDisabled           Key = "ASSET_DISABLED"
	KeyAssetFreezeUnsupported  Key = "ASSET_FREEZE_UNSUPPORTED"
	KeyHolderTypeNotAllowed    Key = "HOLDER_TYPE_NOT_ALLOWED"
	KeyAccountDisabled         Key = "ACCOUNT_DISABLED"
	KeyCaptureExceedsFreeze    Key = "CAPTURE_EXCEEDS_FREEZE"
	KeyTransferNeedsToHolder   Key = "TRANSFER_NEEDS_TO_HOLDER"
	KeyTransferCrossCurrency   Key = "TRANSFER_CROSS_CURRENCY"
	KeyExchangeSameAsset       Key = "EXCHANGE_SAME_ASSET"
	KeyExchangeAmountInvalid   Key = "EXCHANGE_AMOUNT_INVALID"
	KeyExchangeNeedsRate       Key = "EXCHANGE_NEEDS_RATE"
	KeyFxRateMissing           Key = "FX_RATE_MISSING"
	KeyFxRateFormat            Key = "FX_RATE_FORMAT"
	KeyReverseNeedsRelated     Key = "REVERSE_NEEDS_RELATED"
	KeyReverseAlreadyReversed  Key = "REVERSE_ALREADY_REVERSED"
	KeyUnknownJob              Key = "UNKNOWN_JOB"
	KeySagaAlreadyCompleted    Key = "SAGA_ALREADY_COMPLETED"
	KeyUnknownSagaStatus       Key = "UNKNOWN_SAGA_STATUS"
	KeyUnknownTenant           Key = "UNKNOWN_TENANT"
	KeyUnknownEvent            Key = "UNKNOWN_EVENT"
	KeyUnauthorized            Key = "UNAUTHORIZED"
	KeyReplay                  Key = "REPLAY"
	KeyConsoleTokenInQuery     Key = "CONSOLE_TOKEN_IN_QUERY"
	KeyUnknownClient           Key = "UNKNOWN_CLIENT"
	KeyMissingSignature        Key = "MISSING_SIGNATURE"
	KeyTimestampInvalid        Key = "TIMESTAMP_INVALID"
	KeyTimestampSkew           Key = "TIMESTAMP_SKEW"
	KeyUnknownKeyVersion       Key = "UNKNOWN_KEY_VERSION"
	KeySignVersionUnsupported  Key = "SIGN_VERSION_UNSUPPORTED"
	KeyNonceRequired           Key = "NONCE_REQUIRED"
	KeySignatureMismatch       Key = "SIGNATURE_MISMATCH"
	KeyForbidden               Key = "FORBIDDEN"
	KeySourceSystemMismatch    Key = "SOURCE_SYSTEM_MISMATCH"
	KeyTenantHeaderMismatch    Key = "TENANT_HEADER_MISMATCH"
	KeyTenantNotAllowed        Key = "TENANT_NOT_ALLOWED"
	KeyTenantDisabled          Key = "TENANT_DISABLED"
	KeyConsoleRoleDenied       Key = "CONSOLE_ROLE_DENIED"
	KeyNotFound                Key = "NOT_FOUND"
	KeyIdempotencyConflict     Key = "IDEMPOTENCY_CONFLICT"
	KeyInsufficient            Key = "INSUFFICIENT_BALANCE"
	KeyFreezeStateInvalid      Key = "FREEZE_STATE_INVALID"
	KeySlippage                Key = "SLIPPAGE_EXCEEDED"
	KeyCrossShard              Key = "CROSS_SHARD_FORBIDDEN"
	KeyRateLimited             Key = "RATE_LIMITED"
	KeyInternal                Key = "INTERNAL_ERROR"
	KeyOptimisticLock          Key = "OPTIMISTIC_LOCK"
	KeySagaFailed              Key = "SAGA_FAILED"
	KeySagaCompensated         Key = "SAGA_COMPENSATED"
	KeyFreezeLedgerMismatch    Key = "FREEZE_LEDGER_MISMATCH"
	KeyNotImplemented          Key = "NOT_IMPLEMENTED"
	KeyUpstream                Key = "UPSTREAM_ERROR"
	KeyNotReady                Key = "NOT_READY"
)

type Error struct {
	Code    int
	Key     Key
	Message string
	Args    []string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%d %s: %s", e.Code, e.Key, Localize("zh", e))
}

func (e *Error) HTTPStatus() int {
	if e == nil || e.Code <= 0 {
		return 200
	}
	s := e.Code / 100
	if s < 400 || s > 599 {
		return 400
	}
	return s
}

func Keyed(code int, key Key, args ...string) *Error {
	return &Error{Code: code, Key: key, Args: args}
}

func NewError(code int, msg string) *Error {
	return &Error{Code: code, Key: keyForCode(code), Message: msg}
}

func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	if de, ok := err.(*Error); ok && de != nil {
		if de.Key == "" {
			de.Key = keyForCode(de.Code)
		}
		return de
	}
	return Keyed(CodeInternal, KeyInternal)
}

func Is(err error, code int) bool {
	if err == nil {
		return false
	}
	de, ok := err.(*Error)
	return ok && de.Code == code
}

func IsKey(err error, key Key) bool {
	if err == nil {
		return false
	}
	de, ok := err.(*Error)
	return ok && de.Key == key
}

var (
	ErrInvalidParam        = Keyed(CodeInvalidParam, KeyInvalidParam)
	ErrUnauthorized        = Keyed(CodeUnauthorized, KeyUnauthorized)
	ErrReplay              = Keyed(CodeReplay, KeyReplay)
	ErrForbidden           = Keyed(CodeForbidden, KeyForbidden)
	ErrConsoleRoleDenied   = Keyed(CodeConsoleRoleDenied, KeyConsoleRoleDenied)
	ErrNotFound            = Keyed(CodeNotFound, KeyNotFound)
	ErrIdempotencyConflict = Keyed(CodeIdempotencyConflict, KeyIdempotencyConflict)
	ErrInsufficient        = Keyed(CodeInsufficient, KeyInsufficient)
	ErrFreezeStateInvalid  = Keyed(CodeFreezeStateInvalid, KeyFreezeStateInvalid)
	ErrSlippage            = Keyed(CodeSlippage, KeySlippage)
	ErrCrossShard          = Keyed(CodeCrossShard, KeyCrossShard)
	ErrRateLimited         = Keyed(CodeRateLimited, KeyRateLimited)
	ErrNotImplemented      = Keyed(CodeNotImplemented, KeyNotImplemented)
	ErrInternal            = Keyed(CodeInternal, KeyInternal)
)

func keyForCode(code int) Key {
	switch code {
	case CodeJSONInvalid:
		return KeyJSONInvalid
	case CodeAmountNotInteger:
		return KeyAmountNotInteger
	case CodeUnauthorized:
		return KeyUnauthorized
	case CodeReplay:
		return KeyReplay
	case CodeForbidden:
		return KeyForbidden
	case CodeNotFound:
		return KeyNotFound
	case CodeIdempotencyConflict:
		return KeyIdempotencyConflict
	case CodeInsufficient:
		return KeyInsufficient
	case CodeFreezeStateInvalid:
		return KeyFreezeStateInvalid
	case CodeSlippage:
		return KeySlippage
	case CodeCrossShard:
		return KeyCrossShard
	case CodeRateLimited:
		return KeyRateLimited
	case CodeNotImplemented:
		return KeyNotImplemented
	case CodeInternal:
		return KeyInternal
	case CodeUpstream:
		return KeyUpstream
	default:
		if code/100 == 400 {
			return KeyInvalidParam
		}
		if code/100 == 401 {
			return KeyUnauthorized
		}
		if code/100 == 403 {
			return KeyForbidden
		}
		return KeyInternal
	}
}
