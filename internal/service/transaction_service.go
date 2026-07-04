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
	"github.com/ramadiaz/money-wa-bot/internal/shared/logger"
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
	logger.Log.Info().Msg("listing categories for transaction creation match")
	categories, err := s.catCacheRepo.List(ctx)
	if err != nil {
		return 0, err
	}
	logger.Log.Info().Msg("listing accounts for transaction creation match")
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

	logger.Log.Info().Str("hint", categoryHint).Msg("attempting to match transaction category hint")
	matchedCat := MatchCategory(categoryHint, categories)
	if matchedCat == nil {
		logger.Log.Warn().Str("hint", categoryHint).Msg("category hint could not be matched")
		return 0, fmt.Errorf("%w: %s", apperrors.ErrUnknownCategory, categoryHint)
	}
	logger.Log.Info().Str("category_id", matchedCat.CategoryID).Str("title", matchedCat.Title).Msg("matched category successfully")

	categoryID := matchedCat.CategoryID
	accountID := ""
	if accountHint != "" {
		logger.Log.Info().Str("hint", accountHint).Msg("attempting to match transaction account hint")
		matchedAcc := MatchAccount(accountHint, accounts)
		if matchedAcc != nil {
			accountID = matchedAcc.AccountID
			logger.Log.Info().Str("account_id", matchedAcc.AccountID).Str("name", matchedAcc.Name).Msg("matched account successfully")
		} else {
			logger.Log.Warn().Str("hint", accountHint).Msg("account hint specified but could not be matched")
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

	logger.Log.Info().Str("chat_id", chatID).Str("amount", insert.Amount).Msg("inserting pending transaction into database")
	return s.pendingRepo.Insert(ctx, insert)
}

func (s *TransactionService) Commit(ctx context.Context, pendingID int64, pending *transaction.PendingTransactionRow) (*transaction.CreatedTransaction, error) {
	logger.Log.Info().Int64("pending_id", pendingID).Msg("committing pending transaction")

	amount, err := decimal.NewFromString(pending.Amount)
	if err != nil {
		logger.Log.Error().Err(err).Int64("pending_id", pendingID).Msg("parse amount string failed")
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
	logger.Log.Info().Int64("pending_id", pendingID).Msg("inserting transaction submission record into database")
	subID, err := s.submissionRepo.Insert(ctx, &transaction.SubmissionInsert{
		PendingTransactionID: pendingID,
		RequestSnapshotJSON:  string(reqJSON),
	})
	if err != nil {
		logger.Log.Error().Err(err).Int64("pending_id", pendingID).Msg("insert transaction submission failed")
		return nil, fmt.Errorf("insert submission: %w", err)
	}

	logger.Log.Info().Int64("submission_id", subID).Msg("posting transaction payload to money tracker api")
	created, err := s.mtClient.AddTransaction(ctx, req)
	if err != nil {
		logger.Log.Warn().Err(err).Int64("submission_id", subID).Msg("money tracker api call failed, updating submission to failed")
		_ = s.submissionRepo.UpdateFailed(ctx, subID, err.Error())
		return nil, err
	}

	respJSON, _ := json.Marshal(created)
	logger.Log.Info().Int64("submission_id", subID).Str("mt_tx_id", created.ID).Msg("money tracker transaction recorded successfully, updating submission success")
	_ = s.submissionRepo.UpdateSuccess(ctx, subID, created.ID, string(respJSON))
	_ = s.pendingRepo.MarkConfirmed(ctx, pendingID)

	return created, nil
}
