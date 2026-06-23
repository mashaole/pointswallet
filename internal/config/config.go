package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL              string
	MaxOpenConns             int
	MaxIdleConns             int
	ConnMaxLifetime          time.Duration
	ConnMaxIdleTime          time.Duration
	JWTSecret                string
	JWTTTL                   time.Duration
	AdminEmail               string
	AdminPassword            string
	AdminAccountID           string
	HTTPAddr                 string
	MaxRequestBodyBytes      int64
	MaxDecompressedBodyBytes int64
	GzipMinResponseBytes     int
	LogLevel                 string
	RateLimitRPS             float64
	RateLimitBurst           int
	AuthRateLimitRPS         float64
	PaginationDefaultLimit   int
	PaginationMaxLimit       int
	BatchWorkerCount         int
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:              envOr("DATABASE_URL", "postgres://wallet:wallet@localhost:5432/pointswallet?sslmode=disable"),
		MaxOpenConns:             envInt("DB_MAX_OPEN_CONNS", 10),
		MaxIdleConns:             envInt("DB_MAX_IDLE_CONNS", 5),
		ConnMaxLifetime:          envDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
		ConnMaxIdleTime:          envDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
		JWTSecret:                envOr("JWT_SECRET", "change-me-in-dev"),
		JWTTTL:                   envDuration("JWT_TTL", 24*time.Hour),
		AdminEmail:               envOr("ADMIN_EMAIL", "admin@example.com"),
		AdminPassword:            envOr("ADMIN_PASSWORD", "admin123"),
		AdminAccountID:           envOr("ADMIN_ACCOUNT_ID", "admin"),
		HTTPAddr:                 envOr("HTTP_ADDR", ":8080"),
		MaxRequestBodyBytes:      int64(envInt("MAX_REQUEST_BODY_BYTES", 1048576)),
		MaxDecompressedBodyBytes: int64(envInt("MAX_DECOMPRESSED_BODY_BYTES", 1048576)),
		GzipMinResponseBytes:     envInt("GZIP_MIN_RESPONSE_BYTES", 1024),
		LogLevel:                 envOr("LOG_LEVEL", "info"),
		RateLimitRPS:             envFloat("RATE_LIMIT_RPS", 10),
		RateLimitBurst:           envInt("RATE_LIMIT_BURST", 20),
		AuthRateLimitRPS:         envFloat("AUTH_RATE_LIMIT_RPS", 5),
		PaginationDefaultLimit:   envInt("PAGINATION_DEFAULT_LIMIT", 20),
		PaginationMaxLimit:       envInt("PAGINATION_MAX_LIMIT", 100),
		BatchWorkerCount:         envInt("BATCH_WORKER_COUNT", 8),
	}
	if cfg.JWTSecret == "change-me-in-dev" {
		fmt.Println("warning: using default JWT_SECRET; set JWT_SECRET in production")
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return n
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
