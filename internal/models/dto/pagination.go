package dto

import (
	"fmt"

	"pointswallet/internal/models"
)

type PaginationQuery struct {
	Limit  int
	Offset int
}

func (p PaginationQuery) WithDefaults(defaultLimit, maxLimit int) PaginationQuery {
	if p.Limit <= 0 {
		p.Limit = defaultLimit
	}
	if p.Limit > maxLimit {
		p.Limit = maxLimit
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}

func (p PaginationQuery) Validate(maxLimit int) error {
	if p.Limit < 1 || p.Limit > maxLimit {
		return models.ErrFieldRequired("limit")
	}
	if p.Offset < 0 {
		return models.ErrFieldRequired("offset")
	}
	return nil
}

func ParsePagination(limitStr, offsetStr string, defaultLimit, maxLimit int) PaginationQuery {
	p := PaginationQuery{}
	if limitStr != "" {
		var l int
		if _, err := fmt.Sscan(limitStr, &l); err == nil {
			p.Limit = l
		}
	}
	if offsetStr != "" {
		var o int
		if _, err := fmt.Sscan(offsetStr, &o); err == nil {
			p.Offset = o
		}
	}
	return p.WithDefaults(defaultLimit, maxLimit)
}
