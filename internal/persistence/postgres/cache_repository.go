package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
)

type CategoryCacheRepository struct {
	db *pgxpool.Pool
}

func NewCategoryCacheRepository(db *pgxpool.Pool) *CategoryCacheRepository {
	return &CategoryCacheRepository{db: db}
}

func (r *CategoryCacheRepository) Upsert(ctx context.Context, categories []transaction.Category) error {
	now := time.Now()
	for _, cat := range categories {
		_, err := r.db.Exec(ctx, `
			INSERT INTO money_tracker_categories_cache (category_id, title, type, refreshed_at)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (category_id) DO UPDATE SET title=EXCLUDED.title, type=EXCLUDED.type, refreshed_at=EXCLUDED.refreshed_at`,
			cat.CategoryID, cat.Title, cat.Type, now,
		)
		if err != nil {
			return fmt.Errorf("category cache upsert: %w", err)
		}
	}
	return nil
}

func (r *CategoryCacheRepository) List(ctx context.Context) ([]transaction.Category, error) {
	rows, err := r.db.Query(ctx, `SELECT category_id, title, type, refreshed_at FROM money_tracker_categories_cache`)
	if err != nil {
		return nil, fmt.Errorf("category cache list: %w", err)
	}
	defer rows.Close()

	var cats []transaction.Category
	for rows.Next() {
		var c transaction.Category
		if err := rows.Scan(&c.CategoryID, &c.Title, &c.Type, &c.RefreshedAt); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

var _ transaction.CategoryCacheRepository = (*CategoryCacheRepository)(nil)

type AccountCacheRepository struct {
	db *pgxpool.Pool
}

func NewAccountCacheRepository(db *pgxpool.Pool) *AccountCacheRepository {
	return &AccountCacheRepository{db: db}
}

func (r *AccountCacheRepository) Upsert(ctx context.Context, accounts []transaction.Account) error {
	now := time.Now()
	for _, acc := range accounts {
		_, err := r.db.Exec(ctx, `
			INSERT INTO money_tracker_accounts_cache (account_id, name, currency_code, refreshed_at)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (account_id) DO UPDATE SET name=EXCLUDED.name, currency_code=EXCLUDED.currency_code, refreshed_at=EXCLUDED.refreshed_at`,
			acc.AccountID, acc.Name, acc.CurrencyCode, now,
		)
		if err != nil {
			return fmt.Errorf("account cache upsert: %w", err)
		}
	}
	return nil
}

func (r *AccountCacheRepository) List(ctx context.Context) ([]transaction.Account, error) {
	rows, err := r.db.Query(ctx, `SELECT account_id, name, currency_code, refreshed_at FROM money_tracker_accounts_cache`)
	if err != nil {
		return nil, fmt.Errorf("account cache list: %w", err)
	}
	defer rows.Close()

	var accounts []transaction.Account
	for rows.Next() {
		var a transaction.Account
		if err := rows.Scan(&a.AccountID, &a.Name, &a.CurrencyCode, &a.RefreshedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

var _ transaction.AccountCacheRepository = (*AccountCacheRepository)(nil)
