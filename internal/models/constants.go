package models

const (
	RoleMember = "member"
	RoleAdmin  = "admin"
)

const (
	KindEarn       = "earn"
	KindSpend      = "spend"
	KindAdjustment = "adjustment"
)

const (
	DirectionCredit = "credit"
	DirectionDebit  = "debit"
)

const (
	SourceAPI   = "api"
	SourceBatch = "batch"
)

const (
	BatchStatusQueued     = "queued"
	BatchStatusProcessing = "processing"
	BatchStatusCompleted  = "completed"
	BatchStatusFailed     = "failed"
)

const (
	AuditAccepted = "accepted"
	AuditRejected = "rejected"
)

const (
	// IdempotencyKeyHeader is the HTTP header for API transaction idempotency (maps to ledger ref).
	IdempotencyKeyHeader = "Idempotency-Key"
)
