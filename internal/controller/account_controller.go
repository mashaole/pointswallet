package controller

import (
	"net/http"

	"pointswallet/internal/config"
	"pointswallet/internal/models"
	"pointswallet/internal/models/dto"
	walletsvc "pointswallet/internal/service/wallet"
)

type AccountController struct {
	wallet   *walletsvc.Service
	cfg      config.Config
	maxBytes int64
}

func NewAccountController(wallet *walletsvc.Service, cfg config.Config, maxBytes int64) *AccountController {
	return &AccountController{wallet: wallet, cfg: cfg, maxBytes: maxBytes}
}

func (c *AccountController) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateAccountRequest
	if !decodeAndValidateJSON(w, r, c.maxBytes, &req, func(d *dto.CreateAccountRequest) { d.Sanitize() }, func(d *dto.CreateAccountRequest) error { return d.Validate() }) {
		return
	}
	acct, err := c.wallet.CreateAccount(r.Context(), walletsvc.CreateAccountInput{
		AccountID: req.AccountID,
		Name:      req.Name,
		Email:     req.Email,
		Password:  req.Password,
		Role:      req.Role,
	})
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusCreated, accountResponse(acct))
}

func (c *AccountController) List(w http.ResponseWriter, r *http.Request) {
	p := dto.ParsePagination(r.URL.Query().Get("limit"), r.URL.Query().Get("offset"),
		c.cfg.PaginationDefaultLimit, c.cfg.PaginationMaxLimit)
	if err := p.Validate(c.cfg.PaginationMaxLimit); err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	accounts, total, err := c.wallet.ListAccounts(r.Context(), p.Limit, p.Offset)
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	data := make([]map[string]any, 0, len(accounts))
	for _, acct := range accounts {
		data = append(data, accountResponse(acct))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": paginationMeta(p.Limit, p.Offset, total, len(data)),
	})
}

func (c *AccountController) Get(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	acct, err := c.wallet.GetAccount(r.Context(), accountID)
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusOK, accountResponse(acct))
}

func (c *AccountController) Balance(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	balance, err := c.wallet.GetBalance(r.Context(), accountID)
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"account_id":     accountID,
		"balance_points": balance.WholePoints(),
	})
}

func (c *AccountController) MyBalance(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		WriteError(w, MapDomainError(models.ErrUnauthorized))
		return
	}
	balance, err := c.wallet.GetBalance(r.Context(), claims.Sub)
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"account_id":     claims.Sub,
		"balance_points": balance.WholePoints(),
	})
}

func (c *AccountController) UpdateMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		WriteError(w, MapDomainError(models.ErrUnauthorized))
		return
	}
	var req dto.UpdateMemberAccountRequest
	if !decodeAndValidateJSON(w, r, c.maxBytes, &req, func(d *dto.UpdateMemberAccountRequest) { d.Sanitize() }, func(d *dto.UpdateMemberAccountRequest) error { return d.Validate() }) {
		return
	}
	acct, err := c.wallet.UpdateMemberProfile(r.Context(), claims.Sub, req.Name, req.Email)
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusOK, accountResponse(acct))
}

func (c *AccountController) Update(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	var req dto.UpdateAdminAccountRequest
	if !decodeAndValidateJSON(w, r, c.maxBytes, &req, func(d *dto.UpdateAdminAccountRequest) { d.Sanitize() }, func(d *dto.UpdateAdminAccountRequest) error { return d.Validate() }) {
		return
	}
	acct, err := c.wallet.UpdateAccountAsAdmin(r.Context(), accountID, req.Name, req.Email, req.Role)
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusOK, accountResponse(acct))
}

func (c *AccountController) DeleteMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		WriteError(w, MapDomainError(models.ErrUnauthorized))
		return
	}
	if err := c.wallet.SoftDeleteAccount(r.Context(), claims.Sub); err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *AccountController) Delete(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	if err := c.wallet.SoftDeleteAccount(r.Context(), accountID); err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func accountResponse(acct models.Account) map[string]any {
	return map[string]any{
		"account_id":     acct.AccountID,
		"name":           acct.Name,
		"email":          acct.Email,
		"role":           acct.Role,
		"balance_points": acct.BalancePoints.WholePoints(),
	}
}
