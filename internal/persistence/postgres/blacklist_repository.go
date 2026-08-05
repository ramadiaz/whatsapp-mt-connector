package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type BlacklistRepository struct {
	db *gorm.DB
}

func NewBlacklistRepository(db *gorm.DB) *BlacklistRepository {
	return &BlacklistRepository{db: db}
}

func (r *BlacklistRepository) Add(ctx context.Context, phoneNumber, reason, createdBy string) error {
	var existing Blacklist
	err := r.db.WithContext(ctx).Unscoped().Where("phone_number = ?", phoneNumber).First(&existing).Error
	if err == nil {
		return r.db.WithContext(ctx).Model(&existing).Unscoped().Updates(map[string]interface{}{
			"reason":     reason,
			"created_by": createdBy,
			"deleted_at": nil,
		}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	entry := Blacklist{
		PhoneNumber: phoneNumber,
		Reason:      reason,
		CreatedBy:   createdBy,
	}
	return r.db.WithContext(ctx).Create(&entry).Error
}

func (r *BlacklistRepository) Remove(ctx context.Context, phoneNumber string) error {
	return r.db.WithContext(ctx).Where("phone_number = ?", phoneNumber).Delete(&Blacklist{}).Error
}

func (r *BlacklistRepository) IsBlacklisted(ctx context.Context, phoneNumber string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Blacklist{}).Where("phone_number = ?", phoneNumber).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *BlacklistRepository) List(ctx context.Context) ([]string, error) {
	var entries []Blacklist
	err := r.db.WithContext(ctx).Order("created_at desc").Find(&entries).Error
	if err != nil {
		return nil, err
	}
	numbers := make([]string, len(entries))
	for i, e := range entries {
		numbers[i] = e.PhoneNumber
	}
	return numbers, nil
}
