package ninerouter_test

import (
	"testing"

	"github.com/ramadiaz/money-wa-bot/internal/integration/ninerouter"
)

func TestParseAndValidate_Valid(t *testing.T) {
	raw := `{"intent":"create_transaction","type":"expense","amount":25000,"currency_code":"IDR","category_hint":"food","account_hint":null,"date":"2026-07-03","remark":"Kopi susu","confidence":0.98,"needs_confirmation":true,"missing_fields":[]}`
	result, err := ninerouter.ParseAndValidate(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Intent != "create_transaction" {
		t.Errorf("expected create_transaction, got %s", result.Intent)
	}
	if *result.Amount != 25000 {
		t.Errorf("expected 25000, got %f", *result.Amount)
	}
}

func TestParseAndValidate_MarkdownWrapped(t *testing.T) {
	raw := "```json\n{\"intent\":\"create_transaction\",\"type\":\"expense\",\"amount\":25000,\"currency_code\":\"IDR\",\"category_hint\":\"food\",\"account_hint\":null,\"date\":\"2026-07-03\",\"remark\":\"Kopi\",\"confidence\":0.9,\"needs_confirmation\":true,\"missing_fields\":[]}\n```"
	result, err := ninerouter.ParseAndValidate(raw)
	if err != nil {
		t.Fatalf("unexpected error for markdown-wrapped: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
}

func TestParseAndValidate_UnknownField(t *testing.T) {
	raw := `{"intent":"create_transaction","type":"expense","amount":25000,"currency_code":"IDR","category_hint":"food","account_hint":null,"date":"2026-07-03","remark":"x","confidence":0.9,"needs_confirmation":true,"missing_fields":[],"extra_unknown_field":"bad"}`
	_, err := ninerouter.ParseAndValidate(raw)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestParseAndValidate_BadEnum(t *testing.T) {
	raw := `{"intent":"invalid_intent","type":"expense","amount":25000,"currency_code":"IDR","category_hint":"food","account_hint":null,"date":"2026-07-03","remark":"x","confidence":0.9,"needs_confirmation":true,"missing_fields":[]}`
	_, err := ninerouter.ParseAndValidate(raw)
	if err == nil {
		t.Fatal("expected error for invalid intent")
	}
}

func TestParseAndValidate_ZeroAmount(t *testing.T) {
	raw := `{"intent":"create_transaction","type":"expense","amount":0,"currency_code":"IDR","category_hint":"food","account_hint":null,"date":"2026-07-03","remark":"x","confidence":0.9,"needs_confirmation":true,"missing_fields":[]}`
	_, err := ninerouter.ParseAndValidate(raw)
	if err == nil {
		t.Fatal("expected error for amount=0")
	}
}
