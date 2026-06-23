package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pointswallet/internal/config"
	"pointswallet/internal/controller"
	"pointswallet/internal/dao/postgres"
	"pointswallet/internal/router"
	authsvc "pointswallet/internal/service/auth"
	batchsvc "pointswallet/internal/service/batch"
	walletsvc "pointswallet/internal/service/wallet"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}))

	db, err := postgres.OpenPool(cfg)
	if err != nil {
		log.Error("connect db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := postgres.RunMigrations(db); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	walletDAO := postgres.NewWalletDAO(db)
	ledgerDAO := postgres.NewLedgerDAO(db)
	authDAO := postgres.NewAuthDAO(db)
	batchDAO := postgres.NewBatchDAO(db)
	auditDAO := postgres.NewAuditDAO(db)

	authService := authsvc.NewService(authDAO, cfg.JWTSecret, cfg.JWTTTL, cfg.SingleActiveSession)
	walletService := walletsvc.NewService(walletDAO, ledgerDAO, authDAO)
	batchService := batchsvc.NewService(batchDAO, auditDAO, walletService, cfg.BatchWorkerCount)

	ctx := context.Background()
	if err := authService.SeedAdmin(ctx, cfg.AdminAccountID, "Admin", cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Error("seed admin", "err", err)
		os.Exit(1)
	}
	if err := batchService.RecoverStaleJobs(ctx); err != nil {
		log.Error("recover stale jobs", "err", err)
		os.Exit(1)
	}

	handler := router.New(router.Deps{
		Cfg:         cfg,
		Log:         log,
		Auth:        controller.NewAuthController(authService, cfg.MaxRequestBodyBytes),
		Account:     controller.NewAccountController(walletService, cfg, cfg.MaxRequestBodyBytes),
		Transaction: controller.NewTransactionController(walletService, cfg.MaxRequestBodyBytes),
		Ledger:      controller.NewLedgerController(walletService, cfg),
		Batch:       controller.NewBatchController(batchService, cfg),
		AuthService: authService,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("server starting", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info("server stopped")
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
