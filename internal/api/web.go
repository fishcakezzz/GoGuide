package api

import (
	_ "embed"
	"net/http"
)

//go:embed web/index.html
var indexHTML []byte

func (a *Server) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}
