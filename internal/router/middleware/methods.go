package middleware

import (
	"net/http"

	"pointswallet/internal/models"
	"pointswallet/internal/controller"
)

func Methods(allowed ...string) func(http.Handler) http.Handler {
	set := map[string]struct{}{}
	for _, m := range allowed {
		set[m] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := set[r.Method]; !ok {
				controller.WriteError(w, models.NewAPIError("method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
