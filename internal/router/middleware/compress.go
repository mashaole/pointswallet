package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

func Compress(minBytes int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}
			gzw := &gzipResponseWriter{
				ResponseWriter: w,
				minBytes:       minBytes,
				status:         http.StatusOK,
			}
			defer gzw.flush()
			next.ServeHTTP(gzw, r)
		})
	}
}

type gzipResponseWriter struct {
	http.ResponseWriter
	minBytes    int
	buf         bytes.Buffer
	status      int
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.status = statusCode
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader && w.status == 0 {
		w.status = http.StatusOK
	}
	return w.buf.Write(b)
}

func (w *gzipResponseWriter) flush() {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true

	body := w.buf.Bytes()
	if w.status == 0 {
		w.status = http.StatusOK
	}

	if len(body) < w.minBytes {
		w.ResponseWriter.WriteHeader(w.status)
		if len(body) > 0 {
			_, _ = w.ResponseWriter.Write(body)
		}
		return
	}

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(w.status)

	gz := gzip.NewWriter(w.ResponseWriter)
	_, _ = gz.Write(body)
	_ = gz.Close()
}

func Decompress(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Encoding") != "gzip" {
				next.ServeHTTP(w, r)
				return
			}
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "invalid gzip body", http.StatusBadRequest)
				return
			}
			defer zr.Close()
			r.Body = io.NopCloser(io.LimitReader(zr, maxBytes))
			next.ServeHTTP(w, r)
		})
	}
}
