package api

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"gopkg.in/yaml.v3"
)

//go:embed openapi.yaml
var openapiYAML []byte

func (s *Server) openapiJSON(w http.ResponseWriter, r *http.Request) {
	var doc any
	if err := yaml.Unmarshal(openapiYAML, &doc); err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	data, err := json.Marshal(doc)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Server) apiDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><title>Task Control API</title>
<style>body{font-family:system-ui,sans-serif;max-width:760px;margin:40px auto;padding:0 16px}code{background:#f2f2f2;padding:2px 4px}</style>
</head>
<body>
<h1>Task Control API</h1>
<p>OpenAPI JSON: <a href="/api/openapi.json">/api/openapi.json</a></p>
<p>所有业务接口位于 <code>/api/v1</code>，使用 Cookie 会话 + CSRF，或 Bearer API Token。</p>
</body></html>`))
}
