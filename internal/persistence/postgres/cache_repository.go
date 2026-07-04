package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CategoryCacheRepository struct {
	db *gorm.DB
}

func NewCategoryCacheRepository(db *gorm.DB) *CategoryCacheRepository {
	return &CategoryCacheRepository{db: db}
}

func (r *CategoryCacheRepository) Upsert(ctx context.Context, categories []transaction.Category) error {
	now := time.Now()
	for _, cat := range categories {
		cache := CategoryCache{
			CategoryID:  cat.CategoryID,
			Title:       cat.Title,
			Type:        cat.Type,
			RefreshedAt: now,
		}
		err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "category_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "type", "refreshed_at"}),
		}).Create(&cache).Error
		if err != nil {
			return fmt.Errorf("category cache upsert: %w", err)
		}
	}
	return nil
}

func (r *CategoryCacheRepository) List(ctx context.Context) ([]transaction.Category, error) {
	var list []CategoryCache
	err := r.db.WithContext(ctx).Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("category cache list: %w", err)
	}

	cats := make([]transaction.Category, len(list))
	for i, c := range list {
		cats[i] = transaction.Category{
			CategoryID:  c.CategoryID,
			Title:       c.Title,
			Type:        c.Type,
			RefreshedAt: c.RefreshedAt,
		}
	}
	return cats, nil
}

var _ transaction.CategoryCacheRepository = (*CategoryCacheRepository)(nil)

type AccountCacheRepository struct {
	db *gorm.DB
}

func NewAccountCacheRepository(db *gorm.DB) *AccountCacheRepository {
	return &AccountCacheRepository{db: db}
}

func (r *AccountCacheRepository) Upsert(ctx context.Context, accounts []transaction.Account) error {
	now := time.Now()
	for _, acc := range accounts {
		cache := AccountCache{
			AccountID:    acc.AccountID,
			Name:         acc.Name,
			CurrencyCode: acc.CurrencyCode,
			RefreshedAt:  now,
		}
		err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "account_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "currency_code", "refreshed_at"}),
		}).Create(&cache).Error
		if err != nil {
			return fmt.Errorf("account cache upsert: %w", err)
		}
	}
	return nil
}

func (r *AccountCacheRepository) List(ctx context.Context) ([]transaction.Account, error) {
	var list []AccountCache
	err := r.db.WithContext(ctx).Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("account cache list: %w", err)
	}

	accounts := make([]transaction.Account, len(list))
	for i, a := range list {
		accounts[i] = transaction.Account{
			AccountID:    a.AccountID,
			Name:         a.Name,
			CurrencyCode: a.CurrencyCode,
			RefreshedAt:  a.RefreshedAt,
		}
	}
	return accounts, nil
}

var _ transaction.AccountCacheRepository = (*AccountCacheRepository)(nil)
