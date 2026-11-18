package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestVerifyWebhookSignature(t *testing.T) {
	payload, secret := []byte(`{"action":"queued"}`), "test-secret"
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write(payload)
	valid := "sha256=" + hex.EncodeToString(h.Sum(nil))

	for name, signature := range map[string]string{
		"valid": valid, "uppercase hex": "sha256=" + strings.ToUpper(hex.EncodeToString(h.Sum(nil))),
	} {
		t.Run(name, func(t *testing.T) {
			if !VerifyWebhookSignature(payload, signature, secret) {
				t.Fatal("valid signature was rejected")
			}
		})
	}
	for name, signature := range map[string]string{
		"wrong digest": "sha256=" + hex.EncodeToString(make([]byte, sha256.Size)),
		"wrong prefix": valid[1:], "malformed": "sha256=xyz", "empty": "", "whitespace only": "  ",
	} {
		t.Run(name, func(t *testing.T) {
			if VerifyWebhookSignature(payload, signature, secret) {
				t.Fatal("invalid signature was accepted")
			}
		})
	}
	if VerifyWebhookSignature(payload, valid, "") {
		t.Fatal("empty secret was accepted")
	}
}
