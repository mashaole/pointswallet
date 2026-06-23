package middleware

import (
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
			gzw := &gzipResponseWriter{ResponseWriter: w, minBytes: minBytes}
			defer gzw.Close()
			next.ServeHTTP(gzw, r)
		})
	}
}

type gzipResponseWriter struct {
	http.ResponseWriter
	minBytes int
	gz       *gzip.Writer
	wrote    int
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if w.gz == nil {
		if len(b) < w.minBytes {
			return w.ResponseWriter.Write(b)
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.gz = gzip.NewWriter(w.ResponseWriter)
	}
	n, err := w.gz.Write(b)
	w.wrote += n
	return n, err
}

func (w *gzipResponseWriter) Close() {
	if w.gz != nil {
		_ = w.gz.Close()
	}
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
