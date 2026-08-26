package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalorieService_CalculateFromText_Empty(t *testing.T) {
	svc := NewCalorieService(nil, nil, "device-1", 10*1024*1024)
	res, err := svc.CalculateFromText(context.Background(), "")
	assert.NoError(t, err)
	assert.Contains(t, res, "Sebutkan makanan")
}
