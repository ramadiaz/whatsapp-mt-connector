package service_test

import (
	"testing"

	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
	"github.com/ramadiaz/money-wa-bot/internal/service"
)

func TestMatchCategory_FoodKeyword(t *testing.T) {
	cats := []transaction.Category{
		{CategoryID: "cat-food", Title: "category_food", Type: 2},
		{CategoryID: "cat-transport", Title: "category_transport", Type: 2},
	}

	tests := []struct {
		hint     string
		expected string
	}{
		{"kopi", "cat-food"},
		{"makan siang", "cat-food"},
		{"restoran padang", "cat-food"},
		{"bensin motor", "cat-transport"},
		{"parkir", "cat-transport"},
	}

	for _, tt := range tests {
		t.Run(tt.hint, func(t *testing.T) {
			result := service.MatchCategory(tt.hint, cats)
			if result == nil {
				t.Fatalf("expected category for hint %q, got nil", tt.hint)
			}
			if result.CategoryID != tt.expected {
				t.Errorf("hint=%q: expected %q, got %q", tt.hint, tt.expected, result.CategoryID)
			}
		})
	}
}

func TestMatchCategory_NotFound(t *testing.T) {
	cats := []transaction.Category{
		{CategoryID: "cat-food", Title: "category_food", Type: 2},
	}
	result := service.MatchCategory("xyz_unknown_category", cats)
	if result != nil {
		t.Fatalf("expected nil for unknown hint, got %+v", result)
	}
}

func TestMatchCategory_EmptyHint(t *testing.T) {
	cats := []transaction.Category{
		{CategoryID: "cat-food", Title: "category_food", Type: 2},
	}
	result := service.MatchCategory("", cats)
	if result != nil {
		t.Fatal("expected nil for empty hint")
	}
}
