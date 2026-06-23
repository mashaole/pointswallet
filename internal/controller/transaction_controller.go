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
		writeError(w, MapDomainError(models.ErrUnauthorized))
		return
	}
	c.postTransaction(w, r, claims.Sub, claims.Sub)
}

func (c *TransactionController) AdminTransaction(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, MapDomainError(models.ErrUnauthorized))
		return
	}
	accountID := r.PathValue("id")
	c.postTransaction(w, r, accountID, claims.Sub)
}

func (c *TransactionController) postTransaction(w http.ResponseWriter, r *http.Request, accountID, actorID string) {
	var req dto.TransactionRequest
	if !decodeAndValidateJSON(w, r, c.maxBytes, &req, func(d *dto.TransactionRequest) { d.Sanitize() }, req.Validate) {
		return
	}
	occurredAt, err := req.OccurredTime()
	if err != nil {
		writeError(w, MapDomainError(models.ErrValidation))
		return
	}
	entry, err := c.wallet.ApplyTransaction(r.Context(), models.TransactionInput{
		Ref:            req.Ref,
		AccountID:      accountID,
		Kind:           req.Kind,
		WholePoints:    req.Points,
		OccurredAt:     occurredAt,
		ActorAccountID: actorID,
		Source:         models.SourceAPI,
	})
	if err != nil {
		writeError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusCreated, ledgerEntryResponse(entry))
}

func ledgerEntryResponse(e models.LedgerEntry) map[string]any {
	return map[string]any{
		"ref":                  e.Ref,
		"account_id":           e.AccountID,
		"kind":                 e.Kind,
		"points":               e.Points.WholePoints(),
		"balance_after_points": e.BalanceAfterPoints.WholePoints(),
		"occurred_at":          e.OccurredAt,
		"recorded_at":          e.RecordedAt,
		"source":               e.Source,
	}
}
