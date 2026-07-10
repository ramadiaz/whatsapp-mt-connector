package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetOrCreateByPhoneNumber(ctx context.Context, phoneNumber, defaultRole, defaultKey string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("phone_number = ?", phoneNumber).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			user = User{
				PhoneNumber: phoneNumber,
				Role:        defaultRole,
				MTAPIKey:    defaultKey,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			err = r.db.WithContext(ctx).Create(&user).Error
			if err != nil {
				return nil, fmt.Errorf("create user: %w", err)
			}
			return &user, nil
		}
		return nil, fmt.Errorf("find user: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) UpdateAPIKey(ctx context.Context, id int64, apiKey string) error {
	return r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(map[string]interface{}{
		"mt_api_key": apiKey,
		"updated_at": time.Now(),
	}).Error
}

func (r *UserRepository) FindByID(ctx context.Context, id int64) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
