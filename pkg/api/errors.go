package api

type ErrCode string

const (
	ErrNotFound     ErrCode = "not_found"
	ErrConflict     ErrCode = "conflict"
	ErrInvalid      ErrCode = "invalid"
	ErrUnauthorized ErrCode = "unauthorized"
	ErrForbidden    ErrCode = "forbidden"
	ErrInternal     ErrCode = "internal"
	ErrDeployFailed ErrCode = "deploy_failed"
)

type Error struct {
	Code    ErrCode `json:"code"`
	Message string  `json:"message"`
	Details any     `json:"details,omitempty"`
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

func New(code ErrCode, msg string) *Error { return &Error{Code: code, Message: msg} }
