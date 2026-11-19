package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	runtimeconfig "github.com/leo-runners/ci-platform/controller/internal/config"
	"github.com/leo-runners/ci-platform/controller/internal/github"
)

func TestNewGitHubTokenSourceStaticTokenMode(t *testing.T) {
	source, err := newGitHubTokenSource(runtimeconfig.GitHubJITConfig{Enabled: true, InstallationToken: "static-token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := source.(github.StaticBearerToken); !ok {
		t.Fatalf("source type = %T, want StaticBearerToken", source)
	}
}

func TestNewGitHubTokenSourceAppMode(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	jit := runtimeconfig.GitHubJITConfig{
		Enabled: true, AppID: "12345", AppInstallationID: 678, AppPrivateKey: string(keyPEM),
		APIBaseURL: "https://api.github.com", AppTokenPermissions: map[string]string{"administration": "write"},
	}
	source, err := newGitHubTokenSource(jit)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := source.(*github.AppTokenSource); !ok {
		t.Fatalf("source type = %T, want *AppTokenSource", source)
	}
}

func TestNewGitHubTokenSourceRejectsBadKeyWithoutLeakingIt(t *testing.T) {
	secret := "-----BEGIN RSA " + "PRIVATE KEY-----\nsuper-secret-body\n-----END RSA " + "PRIVATE KEY-----"
	_, err := newGitHubTokenSource(runtimeconfig.GitHubJITConfig{
		Enabled: true, AppID: "12345", AppInstallationID: 678, AppPrivateKey: secret, APIBaseURL: "https://api.github.com",
	})
	if err == nil {
		t.Fatal("an unusable private key must fail startup")
	}
	if strings.Contains(err.Error(), "super-secret-body") {
		t.Fatalf("startup error leaks key material: %v", err)
	}
}
