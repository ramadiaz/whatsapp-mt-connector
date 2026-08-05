package blacklist

import "context"

type Repository interface {
	Add(ctx context.Context, phoneNumber, reason, createdBy string) error
	Remove(ctx context.Context, phoneNumber string) error
	IsBlacklisted(ctx context.Context, phoneNumber string) (bool, error)
	List(ctx context.Context) ([]string, error)
}
