package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"seedvault/internal/persistence"
	"seedvault/internal/workflow"
)

func TestWorkspaceAndCreateAPI(t *testing.T) {
	store, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := workflow.NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(service).Routes()
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "种源活力入库核验台") {
		t.Fatalf("页面响应错误: %d", page.Code)
	}
	body := `{"batch_id":"WEB-1","species_name":"小麦","source_region":"甘肃","harvest_date":"2026-01-01","sample_count":400,"storage_condition":"低温","actor":"接收员","role":"receiver","idempotency_key":"web-create-01"}`
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/batches", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), "WEB-1") {
		t.Fatalf("创建响应错误: %d %s", response.Code, response.Body.String())
	}
}
