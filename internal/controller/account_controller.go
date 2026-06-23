package controller

import (
	"net/http"

	"pointswallet/internal/models"
	"pointswallet/internal/models/dto"
	walletsvc "pointswallet/internal/service/wallet"
)

type AccountController struct {
	wallet   *walletsvc.Service
	maxBytes int64
}

func NewAccountController(wallet *walletsvc.Service, maxBytes int64) *AccountController {
	return &AccountController{wallet: wallet, maxBytes: maxBytes}
}

func (c *AccountController) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateAccountRequest
	if !decodeAndValidateJSON(w, r, c.maxBytes, &req, func(d *dto.CreateAccountRequest) { d.Sanitize() }, req.Validate) {
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
		writeError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusCreated, accountResponse(acct))
}

func (c *AccountController) Get(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	acct, err := c.wallet.GetAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusOK, accountResponse(acct))
}

func (c *AccountController) Balance(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	balance, err := c.wallet.GetBalance(r.Context(), accountID)
	if err != nil {
		writeError(w, MapDomainError(err))
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
		writeError(w, MapDomainError(models.ErrUnauthorized))
		return
	}
	balance, err := c.wallet.GetBalance(r.Context(), claims.Sub)
	if err != nil {
		writeError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"account_id":     claims.Sub,
		"balance_points": balance.WholePoints(),
	})
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
