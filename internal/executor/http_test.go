package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPExecutor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	result := HTTP{}.Execute(context.Background(), Context{
		Target: map[string]any{
			"config":   map[string]any{"base_url": server.URL},
			"executor": map[string]any{"path": "/", "method": "GET"},
		},
		Credential:     map[string]any{"token": "token"},
		TimeoutSeconds: 5, OutputLimitBytes: 1024,
	})
	if result.Status != "succeeded" || result.HTTPStatus == nil || *result.HTTPStatus != 200 {
		t.Fatalf("result=%+v class=%v message=%v", result, deref(result.ErrorClass), deref(result.ErrorMessage))
	}
	if result.Response == nil || *result.Response != `{"ok":true}` {
		t.Fatalf("response=%v", result.Response)
	}
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
