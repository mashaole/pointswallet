package middleware

import (
	"net"
	"net/http"
	"sync"

	"golang.org/x/time/rate"

	"pointswallet/internal/controller"
	"pointswallet/internal/models"
)

type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rps      rate.Limit
	burst    int
}

func newIPLimiter(rps float64, burst int) *ipLimiter {
	return &ipLimiter{
		limiters: map[string]*rate.Limiter{},
		rps:      rate.Limit(rps),
		burst:    burst,
	}
}

func (l *ipLimiter) get(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.limiters[ip]
	if !ok {
		lim = rate.NewLimiter(l.rps, l.burst)
		l.limiters[ip] = lim
	}
	return lim
}

func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	limiter := newIPLimiter(rps, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}
			if !limiter.get(ip).Allow() {
				w.Header().Set("Retry-After", "1")
				controller.WriteError(w, models.NewAPIError("rate_limit_exceeded", "Too many requests", http.StatusTooManyRequests))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
