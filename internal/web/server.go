package web

import (
	"embed"
	"io/fs"
	"net/http"
	"time"

	"seedvault/internal/workflow"
)

//go:embed static/*
var staticFiles embed.FS

// Server 是浏览器工作台和 JSON API 的同源适配层。
type Server struct {
	workflow *workflow.Service
	assets   http.Handler
}

func New(service *workflow.Service) *Server {
	sub, _ := fs.Sub(staticFiles, "static")
	return &Server{workflow: service, assets: http.FileServer(http.FS(sub))}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.HandleWorkspace)
	mux.HandleFunc("GET /verify", s.HandleVerifyPage)
	mux.HandleFunc("GET /api/profiles", s.HandleProfiles)
	mux.HandleFunc("GET /api/batches", s.HandleListBatches)
	mux.HandleFunc("POST /api/batches", s.HandleCreateBatch)
	mux.HandleFunc("GET /api/batches/{batchID}", s.HandleGetBatch)
	mux.HandleFunc("GET /api/batches/{batchID}/timeline", s.HandleTimeline)
	mux.HandleFunc("GET /api/batches/{batchID}/evidence-preview", s.HandleEvidencePreview)
	mux.HandleFunc("POST /api/batches/{batchID}/tests", s.HandleRecordTest)
	mux.HandleFunc("POST /api/batches/{batchID}/remediations", s.HandleRemediation)
	mux.HandleFunc("POST /api/batches/{batchID}/reviews", s.HandleReview)
	mux.HandleFunc("POST /api/batches/{batchID}/freeze", s.HandleFreeze)
	mux.HandleFunc("GET /api/credentials/{credentialID}", s.HandleGetCredential)
	mux.HandleFunc("POST /api/credentials/{credentialID}/revoke", s.HandleRevokeCredential)
	mux.HandleFunc("POST /api/credentials/verify", s.HandleVerifyCredential)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", s.assets))
	return securityHeaders(requestLog(mux))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		_ = started
	})
}
