package domain

import (
	"context"
	"time"
)

type TokenRepository interface {
	CountActive(context.Context, int64, time.Time) (int, error)
	Create(context.Context, *Token) error
	ListByUser(context.Context, int64) ([]Token, error)
	Revoke(context.Context, int64, int64, int64, time.Time) (*Token, error)
	Authenticate(context.Context, string, time.Time) (Principal, error)
}
