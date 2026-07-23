package ninerouter_test

import (
	"testing"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/ninerouter"
)

func TestParseAndValidate_Valid(t *testing.T) {
	raw := `{"intent":"create_transaction","type":"expense","amount":25000,"currency_code":"IDR","category_hint":"food","account_hint":null,"date":"2026-07-03","remark":"Kopi susu","confidence":0.98,"needs_confirmation":true,"missing_fields":[],"is_wasteful":true,"wasteful_reason":"Membeli kopi kekinian setiap hari"}`
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
	if result.IsWasteful == nil || !*result.IsWasteful {
		t.Errorf("expected is_wasteful true")
	}
	if result.WastefulReason == nil || *result.WastefulReason != "Membeli kopi kekinian setiap hari" {
		t.Errorf("unexpected wasteful_reason: %v", result.WastefulReason)
	}
}

func TestParseAndValidate_MarkdownWrapped(t *testing.T) {
	raw := "```json\n{\"intent\":\"create_transaction\",\"type\":\"expense\",\"amount\":25000,\"currency_code\":\"IDR\",\"category_hint\":\"food\",\"account_hint\":null,\"date\":\"2026-07-03\",\"remark\":\"Kopi\",\"confidence\":0.9,\"needs_confirmation\":true,\"missing_fields\":[],\"is_wasteful\":false,\"wasteful_reason\":null}\n```"
	result, err := ninerouter.ParseAndValidate(raw)
	if err != nil {
		t.Fatalf("unexpected error for markdown-wrapped: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
}

func TestParseAndValidate_UnknownField(t *testing.T) {
	raw := `{"intent":"create_transaction","type":"expense","amount":25000,"currency_code":"IDR","category_hint":"food","account_hint":null,"date":"2026-07-03","remark":"x","confidence":0.9,"needs_confirmation":true,"missing_fields":[],"is_wasteful":false,"wasteful_reason":null,"extra_unknown_field":"bad"}`
	_, err := ninerouter.ParseAndValidate(raw)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestParseAndValidate_BadEnum(t *testing.T) {
	raw := `{"intent":"invalid_intent","type":"expense","amount":25000,"currency_code":"IDR","category_hint":"food","account_hint":null,"date":"2026-07-03","remark":"x","confidence":0.9,"needs_confirmation":true,"missing_fields":[],"is_wasteful":false,"wasteful_reason":null}`
	_, err := ninerouter.ParseAndValidate(raw)
	if err == nil {
		t.Fatal("expected error for invalid intent")
	}
}

func TestParseAndValidate_ZeroAmount(t *testing.T) {
	raw := `{"intent":"create_transaction","type":"expense","amount":0,"currency_code":"IDR","category_hint":"food","account_hint":null,"date":"2026-07-03","remark":"x","confidence":0.9,"needs_confirmation":true,"missing_fields":[],"is_wasteful":false,"wasteful_reason":null}`
	_, err := ninerouter.ParseAndValidate(raw)
	if err == nil {
		t.Fatal("expected error for amount=0")
	}
}

func TestParseAndValidate_MultiTransaction(t *testing.T) {
	raw := `{"intent":"create_transaction","transactions":[{"type":"expense","amount":20000,"currency_code":"IDR","category_hint":"warkop","account_hint":"supa","date":"2026-07-23","remark":"warkop","confidence":0.95,"needs_confirmation":true,"missing_fields":[],"is_wasteful":false,"wasteful_reason":null},{"type":"expense","amount":2000,"currency_code":"IDR","category_hint":"parkir","account_hint":"cash","date":"2026-07-23","remark":"parkir","confidence":0.95,"needs_confirmation":true,"missing_fields":[],"is_wasteful":false,"wasteful_reason":null}]}`
	result, err := ninerouter.ParseAndValidate(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(result.Transactions))
	}
	if *result.Transactions[0].Amount != 20000 {
		t.Errorf("expected 20000, got %f", *result.Transactions[0].Amount)
	}
	if *result.Transactions[1].Amount != 2000 {
		t.Errorf("expected 2000, got %f", *result.Transactions[1].Amount)
	}
}
