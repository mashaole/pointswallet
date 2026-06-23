package models

const (
	RoleMember = "member"
	RoleAdmin  = "admin"
)

const (
	KindEarn        = "earn"
	KindSpend       = "spend"
	KindAdjustment  = "adjustment"
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
