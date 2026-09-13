package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// VerifyWebhookSignature verifies GitHub's X-Hub-Signature-256 value.
// Signature comparison is constant-time after strict format validation.
func VerifyWebhookSignature(payload []byte, signature, secret string) bool {
	if strings.TrimSpace(secret) == "" {
		return false
	}
	signature = strings.TrimSpace(signature)
	if len(signature) != len("sha256=")+sha256.Size*2 || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	provided, err := hex.DecodeString(signature[len("sha256="):])
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	digest := hmac.New(sha256.New, []byte(secret))
	_, _ = digest.Write(payload)
	return hmac.Equal(provided, digest.Sum(nil))
}

// VerifySignature is the concise package-level API used by callers.
func VerifySignature(secret, signature string, body []byte) bool {
	return VerifyWebhookSignature(body, signature, secret)
}

// VerifyRequest validates the signature header for an HTTP webhook request.
func VerifyRequest(secret string, request *http.Request, body []byte) bool {
	return request != nil && VerifyWebhookSignature(body, request.Header.Get("X-Hub-Signature-256"), secret)
}
