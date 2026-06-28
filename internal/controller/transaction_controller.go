package controller

import (
	"net/http"

	"pointswallet/internal/models"
	"pointswallet/internal/models/dto"
	walletsvc "pointswallet/internal/service/wallet"
)

type TransactionController struct {
	wallet   *walletsvc.Service
	maxBytes int64
}

func NewTransactionController(wallet *walletsvc.Service, maxBytes int64) *TransactionController {
	return &TransactionController{wallet: wallet, maxBytes: maxBytes}
}

func (c *TransactionController) MemberTransaction(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		WriteError(w, MapDomainError(models.ErrUnauthorized))
		return
	}
	c.postTransaction(w, r, claims.Sub, claims.Sub, true)
}

func (c *TransactionController) AdminTransaction(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		WriteError(w, MapDomainError(models.ErrUnauthorized))
		return
	}
	accountID := r.PathValue("id")
	c.postTransaction(w, r, accountID, claims.Sub, false)
}

func (c *TransactionController) postTransaction(w http.ResponseWriter, r *http.Request, accountID, actorID string, memberOnly bool) {
	var req dto.TransactionRequest
	if !decodeAndValidateJSON(w, r, c.maxBytes, &req, func(d *dto.TransactionRequest) { d.Sanitize() }, nil) {
		return
	}
	if memberOnly && req.Kind == models.KindAdjustment {
		WriteError(w, MapDomainError(models.ErrForbidden))
		return
	}
	ref, err := dto.ResolveTransactionRef(r.Header.Get(models.IdempotencyKeyHeader), req.Ref)
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	if err := req.ValidateWithResolvedRef(ref); err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	direction, err := req.ResolvedDirection()
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	occurredAt, err := req.OccurredTime()
	if err != nil {
		WriteError(w, MapDomainError(models.ErrValidation))
		return
	}
	entry, err := c.wallet.ApplyTransaction(r.Context(), models.TransactionInput{
		Ref:            ref,
		AccountID:      accountID,
		Kind:           req.Kind,
		Direction:      direction,
		WholePoints:    req.Points,
		OccurredAt:     occurredAt,
		ActorAccountID: actorID,
		Source:         models.SourceAPI,
	})
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusCreated, ledgerEntryResponse(entry))
}

func ledgerEntryResponse(e models.LedgerEntry) map[string]any {
	return map[string]any{
		"ref":                  e.Ref,
		"account_id":           e.AccountID,
		"kind":                 e.Kind,
		"direction":            models.LedgerEntryDirection(e.Kind, e.Direction),
		"points":               e.Points.WholePoints(),
		"balance_after_points": e.BalanceAfterPoints.WholePoints(),
		"occurred_at":          e.OccurredAt,
		"recorded_at":          e.RecordedAt,
		"actor_account_id":     e.ActorAccountID,
		"source":               e.Source,
	}
}
