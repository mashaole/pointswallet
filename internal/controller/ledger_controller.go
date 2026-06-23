package controller

import (
	"net/http"

	"pointswallet/internal/config"
	"pointswallet/internal/models"
	"pointswallet/internal/models/dto"
	walletsvc "pointswallet/internal/service/wallet"
)

type LedgerController struct {
	wallet *walletsvc.Service
	cfg    config.Config
}

func NewLedgerController(wallet *walletsvc.Service, cfg config.Config) *LedgerController {
	return &LedgerController{wallet: wallet, cfg: cfg}
}

func (c *LedgerController) MyLedger(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		WriteError(w, MapDomainError(models.ErrUnauthorized))
		return
	}
	c.listLedger(w, r, claims.Sub)
}

func (c *LedgerController) AccountLedger(w http.ResponseWriter, r *http.Request) {
	c.listLedger(w, r, r.PathValue("id"))
}

func (c *LedgerController) listLedger(w http.ResponseWriter, r *http.Request, accountID string) {
	p := dto.ParsePagination(r.URL.Query().Get("limit"), r.URL.Query().Get("offset"),
		c.cfg.PaginationDefaultLimit, c.cfg.PaginationMaxLimit)
	if err := p.Validate(c.cfg.PaginationMaxLimit); err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	entries, total, err := c.wallet.ListLedger(r.Context(), accountID, p.Limit, p.Offset)
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	data := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		data = append(data, ledgerEntryResponse(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": paginationMeta(p.Limit, p.Offset, total, len(data)),
	})
}

func paginationMeta(limit, offset, total, count int) map[string]any {
	return map[string]any{
		"limit":       limit,
		"offset":      offset,
		"total_count": total,
		"has_more":    offset+count < total,
	}
}
