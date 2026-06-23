package middleware

import (
	"net/http"
	"strings"

	"pointswallet/internal/controller"
	"pointswallet/internal/models"
	authsvc "pointswallet/internal/service/auth"
)

type AuthConfig struct {
	Secret string
	Auth   *authsvc.Service
}

func Auth(cfg AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := authsvc.ParseBearer(r.Header.Get("Authorization"))
			if err != nil {
				controller.WriteError(w, models.NewAPIError("unauthorized", "Unauthorized", http.StatusUnauthorized))
				return
			}
			claims, err := authsvc.VerifyToken(cfg.Secret, token)
			if err != nil {
				controller.WriteError(w, models.NewAPIError("unauthorized", "Unauthorized", http.StatusUnauthorized))
				return
			}
			if err := cfg.Auth.ValidateSession(r.Context(), claims); err != nil {
				controller.WriteError(w, models.NewAPIError("unauthorized", "Unauthorized", http.StatusUnauthorized))
				return
			}
			ctx := controller.WithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := controller.ClaimsFromContext(r.Context())
		if !ok || claims.Role != models.RoleAdmin {
			controller.WriteError(w, models.NewAPIError("forbidden", "Forbidden", http.StatusForbidden))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := controller.ClaimsFromContext(r.Context())
		if !ok || claims.Role != models.RoleMember {
			controller.WriteError(w, models.NewAPIError("forbidden", "Forbidden", http.StatusForbidden))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "application/json") {
			controller.WriteError(w, models.NewAPIError("unsupported_media_type", "Content-Type must be application/json", http.StatusUnsupportedMediaType))
			return
		}
		next.ServeHTTP(w, r)
	})
}
