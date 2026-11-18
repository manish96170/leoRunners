package github

import (
	"testing"
)

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"action":"queued","workflow_job":{"id":1}}`)
	if VerifySignature("secret", "sha256=bad", body) {
		t.Fatal("accepted bad signature")
	}
}
