package web

import "net/http"

func (s *Server) HandleWorkspace(w http.ResponseWriter, r *http.Request) {
	serveEmbedded(w, r, "static/index.html")
}

func (s *Server) HandleVerifyPage(w http.ResponseWriter, r *http.Request) {
	serveEmbedded(w, r, "static/verify.html")
}

func serveEmbedded(w http.ResponseWriter, r *http.Request, name string) {
	data, err := staticFiles.ReadFile(name)
	if err != nil {
		http.Error(w, "页面资源不可用", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}
