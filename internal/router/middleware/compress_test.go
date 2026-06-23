package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompress_largeJSONIsValidGzip(t *testing.T) {
	t.Parallel()

	const minBytes = 64
	body := strings.Repeat(`{"ref":"tx-001","kind":"earn"}`, 8) // > minBytes

	handler := Compress(minBytes)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))

	req := httptest.NewRequest(http.MethodGet, "/accounts/member-1/ledger", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected Content-Encoding gzip, got %q", rec.Header().Get("Content-Encoding"))
	}

	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("invalid gzip body: %v", err)
	}
	defer zr.Close()

	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gzip: %v", err)
	}
	if string(got) != body {
		t.Fatalf("decompressed body mismatch")
	}
}

func TestCompress_smallResponseUncompressed(t *testing.T) {
	t.Parallel()

	handler := Compress(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatalf("small response should not be gzip encoded")
	}
	if rec.Body.String() != `{"status":"ok"}` {
		t.Fatalf("unexpected body %q", rec.Body.String())
	}
}
