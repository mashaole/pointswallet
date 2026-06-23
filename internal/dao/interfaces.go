package dao

import (
	"context"
	"time"

	"pointswallet/internal/models"
)

type WalletDAO interface {
	CreateAccount(ctx context.Context, acct models.Account) error
	GetAccount(ctx context.Context, accountID string) (models.Account, error)
	GetBalance(ctx context.Context, accountID string) (models.Points, error)
	ApplyTransaction(ctx context.Context, in models.TransactionInput) (models.LedgerEntry, error)
}

type LedgerDAO interface {
	ListByAccount(ctx context.Context, accountID string, limit, offset int) ([]models.LedgerEntry, int, error)
	RefExists(ctx context.Context, ref string) (bool, error)
}

type AuthDAO interface {
	GetAccountByEmail(ctx context.Context, email string) (models.Account, error)
	RevokeAllSessions(ctx context.Context, accountID string) error
	CreateSession(ctx context.Context, sessionID, accountID, jti string, expiresAt time.Time) error
	GetActiveSession(ctx context.Context, jti string) (accountID string, err error)
	RevokeSession(ctx context.Context, jti string) error
	CreateResetToken(ctx context.Context, id, accountID, tokenHash string, expiresAt time.Time) error
	UseResetToken(ctx context.Context, tokenHash string) (accountID string, err error)
	UpdatePassword(ctx context.Context, accountID, passwordHash string) error
	SeedAdminIfMissing(ctx context.Context, acct models.Account) error
}

type BatchDAO interface {
	CreateJob(ctx context.Context, id string, totalRows int) error
	UpdateJobStatus(ctx context.Context, id, status string) error
	CompleteJob(ctx context.Context, id string, summary models.BatchSummary) error
	GetJob(ctx context.Context, id string) (BatchJob, error)
	RecoverStaleJobs(ctx context.Context) error
}

type AuditDAO interface {
	InsertAuditEvent(ctx context.Context, batchID, ref, accountID, status, reason string) error
	ListByBatchID(ctx context.Context, batchID string, limit, offset int) ([]AuditEvent, int, error)
}

type BatchJob struct {
	ID              string
	Status          string
	TotalRows       int
	AcceptedCount   int
	RejectedCount   int
	DuplicateCount  int
	CreatedAt       time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	ErrorMessage    *string
}

type AuditEvent struct {
	ID        int64
	BatchID   string
	Ref       string
	AccountID string
	Status    string
	Reason    string
	CreatedAt time.Time
}
