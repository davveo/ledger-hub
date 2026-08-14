package domain

import "fmt"

const (
	CodeOK                  = 0
	CodeInvalidParam        = 40001
	CodeForbidden           = 40301
	CodeNotFound            = 40401
	CodeIdempotencyConflict = 40901
	CodeInsufficient        = 42201
	CodeFreezeStateInvalid  = 42202
	CodeRateLimited         = 42901
	CodeNotImplemented      = 50101
	CodeInternal            = 50000
)

type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

func NewError(code int, msg string) *Error {
	return &Error{Code: code, Message: msg}
}

func Is(err error, code int) bool {
	if err == nil {
		return false
	}
	de, ok := err.(*Error)
	return ok && de.Code == code
}

var (
	ErrInvalidParam        = NewError(CodeInvalidParam, "参数错误 / 资产未注册")
	ErrForbidden           = NewError(CodeForbidden, "无权对该资产执行该命令")
	ErrNotFound            = NewError(CodeNotFound, "账户/冻结单不存在")
	ErrIdempotencyConflict = NewError(CodeIdempotencyConflict, "幂等冲突但命令参数不一致")
	ErrInsufficient        = NewError(CodeInsufficient, "余额不足")
	ErrFreezeStateInvalid  = NewError(CodeFreezeStateInvalid, "冻结单状态不允许 Capture/Release")
	ErrRateLimited         = NewError(CodeRateLimited, "触发限额")
	ErrNotImplemented      = NewError(CodeNotImplemented, "能力尚未交付")
	ErrInternal            = NewError(CodeInternal, "内部错误")
)
