package service_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/service"
)

func hmacSign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookService_VerifySignature_Valid(t *testing.T) {
	svc := service.NewWebhookService("mysecret", []string{"628111"}, "device1", nil, nil)
	body := []byte(`{"event":"message"}`)
	sig := hmacSign("mysecret", body)
	if !svc.VerifySignature(sig, body) {
		t.Fatal("expected valid signature")
	}
}

func TestWebhookService_VerifySignature_Tampered(t *testing.T) {
	svc := service.NewWebhookService("mysecret", []string{"628111"}, "device1", nil, nil)
	body := []byte(`{"event":"message"}`)
	sig := hmacSign("mysecret", body)
	tampered := []byte(`{"event":"tampered"}`)
	if svc.VerifySignature(sig, tampered) {
		t.Fatal("expected invalid signature for tampered body")
	}
}

func TestWebhookService_VerifySignature_WrongSecret(t *testing.T) {
	svc := service.NewWebhookService("mysecret", []string{"628111"}, "device1", nil, nil)
	body := []byte(`{"event":"message"}`)
	sig := hmacSign("wrongsecret", body)
	if svc.VerifySignature(sig, body) {
		t.Fatal("expected invalid signature for wrong secret")
	}
}

func TestWebhookService_VerifySignature_Empty(t *testing.T) {
	svc := service.NewWebhookService("mysecret", []string{"628111"}, "device1", nil, nil)
	if svc.VerifySignature("", []byte("body")) {
		t.Fatal("expected invalid for empty signature")
	}
}
