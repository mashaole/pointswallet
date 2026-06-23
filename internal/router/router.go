package router

import (
	"log/slog"
	"net/http"

	"pointswallet/internal/config"
	"pointswallet/internal/controller"
	authsvc "pointswallet/internal/service/auth"
	"pointswallet/internal/router/middleware"
)

type Deps struct {
	Cfg         config.Config
	Log         *slog.Logger
	Auth        *controller.AuthController
	Account     *controller.AccountController
	Transaction *controller.TransactionController
	Ledger      *controller.LedgerController
	Batch       *controller.BatchController
	AuthService *authsvc.Service
}

func New(d Deps) http.Handler {
	mux := http.NewServeMux()

	chain := func(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
		all := append([]func(http.Handler) http.Handler{
			middleware.Logging(d.Log),
			middleware.RateLimit(d.Cfg.RateLimitRPS, d.Cfg.RateLimitBurst),
		}, mws...)
		return middleware.Chain(h, all...)
	}

	authChain := func(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
		all := append([]func(http.Handler) http.Handler{
			middleware.Logging(d.Log),
			middleware.RateLimit(d.Cfg.AuthRateLimitRPS, d.Cfg.RateLimitBurst),
		}, mws...)
		return middleware.Chain(h, all...)
	}

	authMW := middleware.Auth(middleware.AuthConfig{Secret: d.Cfg.JWTSecret, Auth: d.AuthService})

	mux.Handle("POST /auth/login", authChain(
		http.HandlerFunc(d.Auth.Login),
		middleware.Methods(http.MethodPost),
		middleware.RequireJSON,
	))
	mux.Handle("POST /auth/logout", chain(
		http.HandlerFunc(d.Auth.Logout),
		middleware.Methods(http.MethodPost),
		authMW,
	))
	mux.Handle("POST /auth/forgot-password", authChain(
		http.HandlerFunc(d.Auth.ForgotPassword),
		middleware.Methods(http.MethodPost),
		middleware.RequireJSON,
	))
	mux.Handle("POST /auth/reset-password", authChain(
		http.HandlerFunc(d.Auth.ResetPassword),
		middleware.Methods(http.MethodPost),
		middleware.RequireJSON,
	))

	mux.Handle("GET /accounts", chain(
		http.HandlerFunc(d.Account.List),
		middleware.Methods(http.MethodGet),
		authMW,
		middleware.RequireAdmin,
	))
	mux.Handle("POST /accounts", chain(
		http.HandlerFunc(d.Account.Create),
		middleware.Methods(http.MethodPost),
		middleware.RequireJSON,
		authMW,
		middleware.RequireAdmin,
	))
	mux.Handle("PATCH /accounts/{id}", chain(
		http.HandlerFunc(d.Account.Update),
		middleware.Methods(http.MethodPatch),
		middleware.RequireJSON,
		authMW,
		middleware.RequireAdmin,
	))
	mux.Handle("DELETE /accounts/{id}", chain(
		http.HandlerFunc(d.Account.Delete),
		middleware.Methods(http.MethodDelete),
		authMW,
		middleware.RequireAdmin,
	))
	mux.Handle("GET /accounts/{id}", chain(
		http.HandlerFunc(d.Account.Get),
		middleware.Methods(http.MethodGet),
		authMW,
		middleware.RequireAdmin,
	))
	mux.Handle("GET /accounts/{id}/balance", chain(
		http.HandlerFunc(d.Account.Balance),
		middleware.Methods(http.MethodGet),
		authMW,
		middleware.RequireAdmin,
	))
	mux.Handle("GET /accounts/{id}/ledger", chain(
		http.HandlerFunc(d.Ledger.AccountLedger),
		middleware.Methods(http.MethodGet),
		authMW,
		middleware.RequireAdmin,
	))
	mux.Handle("POST /accounts/{id}/transactions", chain(
		http.HandlerFunc(d.Transaction.AdminTransaction),
		middleware.Methods(http.MethodPost),
		middleware.RequireJSON,
		authMW,
		middleware.RequireAdmin,
	))

	mux.Handle("GET /accounts/me/balance", chain(
		http.HandlerFunc(d.Account.MyBalance),
		middleware.Methods(http.MethodGet),
		authMW,
		middleware.RequireMember,
	))
	mux.Handle("PATCH /accounts/me", chain(
		http.HandlerFunc(d.Account.UpdateMe),
		middleware.Methods(http.MethodPatch),
		middleware.RequireJSON,
		authMW,
		middleware.RequireMember,
	))
	mux.Handle("DELETE /accounts/me", chain(
		http.HandlerFunc(d.Account.DeleteMe),
		middleware.Methods(http.MethodDelete),
		authMW,
		middleware.RequireMember,
	))
	mux.Handle("GET /accounts/me/ledger", chain(
		http.HandlerFunc(d.Ledger.MyLedger),
		middleware.Methods(http.MethodGet),
		authMW,
		middleware.RequireMember,
	))
	mux.Handle("POST /transactions", chain(
		http.HandlerFunc(d.Transaction.MemberTransaction),
		middleware.Methods(http.MethodPost),
		middleware.RequireJSON,
		authMW,
		middleware.RequireMember,
	))

	mux.Handle("POST /batch/transactions", chain(
		http.HandlerFunc(d.Batch.Upload),
		middleware.Methods(http.MethodPost),
		authMW,
		middleware.RequireAdmin,
	))
	mux.Handle("GET /batch/jobs/{id}", chain(
		http.HandlerFunc(d.Batch.GetJob),
		middleware.Methods(http.MethodGet),
		authMW,
		middleware.RequireAdmin,
	))
	mux.Handle("GET /batch/jobs/{id}/audit", chain(
		http.HandlerFunc(d.Batch.Audit),
		middleware.Methods(http.MethodGet),
		authMW,
		middleware.RequireAdmin,
	))

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	handler := middleware.Compress(d.Cfg.GzipMinResponseBytes)(mux)
	handler = middleware.Decompress(d.Cfg.MaxDecompressedBodyBytes)(handler)
	return handler
}
