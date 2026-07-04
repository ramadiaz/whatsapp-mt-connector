package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
	"github.com/ramadiaz/money-wa-bot/internal/integration/moneytracker"
	"github.com/ramadiaz/money-wa-bot/internal/integration/ninerouter"
	apperrors "github.com/ramadiaz/money-wa-bot/internal/shared/errors"
	"github.com/ramadiaz/money-wa-bot/internal/shared/timeutil"
)

type TransactionService struct {
	mtClient      moneytracker.MoneyTrackerClient
	catCacheRepo  transaction.CategoryCacheRepository
	accCacheRepo  transaction.AccountCacheRepository
	pendingRepo   transaction.PendingTransactionRepository
	submissionRepo transaction.SubmissionRepository
}

func NewTransactionService(
	mtClient moneytracker.MoneyTrackerClient,
	catCacheRepo transaction.CategoryCacheRepository,
	accCacheRepo transaction.AccountCacheRepository,
	pendingRepo transaction.PendingTransactionRepository,
	submissionRepo transaction.SubmissionRepository,
) *TransactionService {
	return &TransactionService{
		mtClient:       mtClient,
		catCacheRepo:   catCacheRepo,
		accCacheRepo:   accCacheRepo,
		pendingRepo:    pendingRepo,
		submissionRepo: submissionRepo,
	}
}

func (s *TransactionService) CreatePending(ctx context.Context, chatID, sourceMessageID string, result *ninerouter.AIExtractionResult) (int64, error) {
	categories, err := s.catCacheRepo.List(ctx)
	if err != nil {
		return 0, err
	}
	accounts, err := s.accCacheRepo.List(ctx)
	if err != nil {
		return 0, err
	}

	categoryHint := ""
	if result.CategoryHint != nil {
		categoryHint = *result.CategoryHint
	}

	accountHint := ""
	if result.AccountHint != nil {
		accountHint = *result.AccountHint
	}

	matchedCat := MatchCategory(categoryHint, categories)
	if matchedCat == nil {
		return 0, fmt.Errorf("%w: %s", apperrors.ErrUnknownCategory, categoryHint)
	}

	categoryID := matchedCat.CategoryID
	accountID := ""
	if accountHint != "" {
		matchedAcc := MatchAccount(accountHint, accounts)
		if matchedAcc != nil {
			accountID = matchedAcc.AccountID
		}
	}

	txType := "expense"
	if result.Type != nil {
		txType = *result.Type
	}

	date := timeutil.TodayJakarta()
	if result.Date != nil && *result.Date != "" {
		date = *result.Date
	}

	remark := ""
	if result.Remark != nil {
		remark = *result.Remark
	}

	amount := decimal.NewFromFloat(*result.Amount)

	insert := &transaction.PendingTransactionInsert{
		ChatID:          chatID,
		SourceMessageID: sourceMessageID,
		Type:            txType,
		Amount:          amount.String(),
		CurrencyCode:    result.CurrencyCode,
		CategoryHint:    categoryHint,
		CategoryID:      categoryID,
		AccountHint:     accountHint,
		AccountID:       accountID,
		TransactionDate: date,
		Remark:          remark,
		Confidence:      result.Confidence,
	}

	return s.pendingRepo.Insert(ctx, insert)
}

func (s *TransactionService) Commit(ctx context.Context, pendingID int64, pending *transaction.PendingTransactionRow) (*transaction.CreatedTransaction, error) {
	amount, err := decimal.NewFromString(pending.Amount)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	txType := transaction.TypeExpense
	if pending.Type == "income" {
		txType = transaction.TypeIncome
	}

	req := transaction.CreateTransactionRequest{
		Type:         txType,
		Amount:       amount,
		CategoryID:   pending.CategoryID,
		AccountID:    pending.AccountID,
		Date:         pending.TransactionDate,
		Remark:       pending.Remark,
		CurrencyCode: pending.CurrencyCode,
	}

	reqJSON, _ := json.Marshal(req)
	subID, err := s.submissionRepo.Insert(ctx, &transaction.SubmissionInsert{
		PendingTransactionID: pendingID,
		RequestSnapshotJSON:  string(reqJSON),
	})
	if err != nil {
		return nil, fmt.Errorf("insert submission: %w", err)
	}

	created, err := s.mtClient.AddTransaction(ctx, req)
	if err != nil {
		_ = s.submissionRepo.UpdateFailed(ctx, subID, err.Error())
		return nil, err
	}

	respJSON, _ := json.Marshal(created)
	_ = s.submissionRepo.UpdateSuccess(ctx, subID, created.ID, string(respJSON))
	_ = s.pendingRepo.MarkConfirmed(ctx, pendingID)

	return created, nil
}
