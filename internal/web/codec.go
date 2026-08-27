package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"seedvault/internal/workflow"
)

type envelope struct {
	Data  any       `json:"data,omitempty"`
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "JSON 请求无效: "+err.Error(), nil)
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "请求只能包含一个 JSON 对象", nil)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Data: data})
}

func writeDomainError(w http.ResponseWriter, err error) {
	var domain *workflow.DomainError
	if !errors.As(err, &domain) {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "服务处理失败", nil)
		return
	}
	status := http.StatusBadRequest
	switch domain.Code {
	case workflow.CodeNotFound:
		status = http.StatusNotFound
	case workflow.CodeConflict:
		status = http.StatusConflict
	case workflow.CodeForbidden:
		status = http.StatusForbidden
	case workflow.CodePersistence:
		status = http.StatusInternalServerError
	case workflow.CodeState:
		status = http.StatusUnprocessableEntity
	}
	writeError(w, status, string(domain.Code), domain.Message, domain.Fields)
}

func writeError(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Error: &apiError{Code: code, Message: message, Fields: fields}})
}

func requestKey(r *http.Request, bodyKey string) string {
	if header := r.Header.Get("Idempotency-Key"); header != "" {
		return header
	}
	return bodyKey
}
