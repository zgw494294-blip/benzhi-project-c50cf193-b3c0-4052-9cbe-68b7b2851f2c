package workflow

import "fmt"

type ErrorCode string

const (
	CodeInvalid     ErrorCode = "INVALID_ARGUMENT"
	CodeNotFound    ErrorCode = "NOT_FOUND"
	CodeConflict    ErrorCode = "VERSION_CONFLICT"
	CodeForbidden   ErrorCode = "FORBIDDEN"
	CodeState       ErrorCode = "INVALID_STATE"
	CodePersistence ErrorCode = "PERSISTENCE_ERROR"
)

// DomainError 供 Web 层稳定映射 HTTP 状态和字段错误。
type DomainError struct {
	Code    ErrorCode         `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *DomainError) Error() string { return e.Message }

func invalid(field, message string) error {
	return &DomainError{Code: CodeInvalid, Message: message, Fields: map[string]string{field: message}}
}

func stateError(status Status, action string) error {
	return &DomainError{Code: CodeState, Message: fmt.Sprintf("状态 %s 不允许%s", status, action)}
}

func conflict(expected, actual int) error {
	return &DomainError{Code: CodeConflict, Message: fmt.Sprintf("版本冲突：期望 %d，当前 %d", expected, actual), Fields: map[string]string{"expected_version": "请刷新批次后重试"}}
}
