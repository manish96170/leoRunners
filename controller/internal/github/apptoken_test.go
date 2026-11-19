package github

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testKey is generated once; the key exists only for the test process and is
// never written anywhere.
var (
	testKeyOnce sync.Once
	testKeyVal  *rsa.PrivateKey
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	testKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		testKeyVal = key
	})
	return testKeyVal
}

func pkcs1PEM(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func pkcs8PEM(t *testing.T, key any) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type fakeGitHub struct {
	t        *testing.T
	pub      *rsa.PublicKey
	clock    *fakeClock
	mints    atomic.Int64
	status   atomic.Int64
	lastBody atomic.Value
	lifetime time.Duration
}

func newFakeGitHub(t *testing.T, key *rsa.PrivateKey, clock *fakeClock) (*fakeGitHub, *httptest.Server) {
	f := &fakeGitHub{t: t, pub: &key.PublicKey, clock: clock, lifetime: time.Hour}
	f.status.Store(http.StatusCreated)
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	return f, server
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/app/installations/4242/access_tokens" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	f.mints.Add(1)
	if err := f.verifyJWT(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); err != nil {
		f.t.Errorf("invalid app JWT: %v", err)
		http.Error(w, "bad jwt", http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
	f.lastBody.Store(string(body))
	if code := int(f.status.Load()); code != http.StatusCreated {
		http.Error(w, `{"message":"Bad credentials"}`, code)
		return
	}
	n := f.mints.Load()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":      "ghs_installation_" + string(rune('a'+n)),
		"expires_at": f.clock.Now().Add(f.lifetime).UTC().Format(time.RFC3339),
	})
}

func (f *fakeGitHub) verifyJWT(token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("not a three part token")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}
	var h map[string]string
	if err := json.Unmarshal(header, &h); err != nil || h["alg"] != "RS256" {
		return errors.New("alg is not RS256")
	}
	claims, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	var c struct {
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(claims, &c); err != nil {
		return err
	}
	now := f.clock.Now()
	if c.Iss != "12345" {
		return errors.New("unexpected issuer")
	}
	if c.Iat > now.Unix() {
		return errors.New("iat is in the future")
	}
	if c.Exp <= now.Unix() || c.Exp-now.Unix() > 600 {
		return errors.New("exp must be within 10 minutes")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	return rsa.VerifyPKCS1v15(f.pub, crypto.SHA256, digest[:], signature)
}

func newTestSource(t *testing.T, serverURL string, clock *fakeClock, mutate func(*AppTokenSourceConfig)) *AppTokenSource {
	t.Helper()
	config := AppTokenSourceConfig{
		AppID: "12345", InstallationID: 4242, PrivateKeyPEM: pkcs1PEM(testKey(t)), BaseURL: serverURL,
		Now: clock.Now, Retry: RetryConfig{MaxAttempts: 1},
	}
	if mutate != nil {
		mutate(&config)
	}
	source, err := NewAppTokenSource(config)
	if err != nil {
		t.Fatalf("NewAppTokenSource() error = %v", err)
	}
	return source
}

func TestAppTokenSourceMintsAndCaches(t *testing.T) {
	clock := &fakeClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	fake, server := newFakeGitHub(t, testKey(t), clock)
	source := newTestSource(t, server.URL, clock, nil)

	first, err := source.BearerToken(context.Background())
	if err != nil || !strings.HasPrefix(first, "ghs_installation_") {
		t.Fatalf("BearerToken() = %q, %v", first, err)
	}
	clock.Advance(30 * time.Minute)
	second, err := source.BearerToken(context.Background())
	if err != nil || second != first {
		t.Fatalf("cached BearerToken() = %q, %v; want %q", second, err, first)
	}
	if got := fake.mints.Load(); got != 1 {
		t.Fatalf("mint calls = %d, want 1", got)
	}
	if body, _ := fake.lastBody.Load().(string); body != "" {
		t.Fatalf("request body = %q, want none when no permissions are set", body)
	}
}

func TestAppTokenSourceRefreshesBeforeExpiry(t *testing.T) {
	clock := &fakeClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	fake, server := newFakeGitHub(t, testKey(t), clock)
	source := newTestSource(t, server.URL, clock, nil)

	first, err := source.BearerToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(56 * time.Minute) // inside the 5 minute refresh window of a 1 hour token
	second, err := source.BearerToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second == first || fake.mints.Load() != 2 {
		t.Fatalf("token was not refreshed: first=%q second=%q mints=%d", first, second, fake.mints.Load())
	}
}

func TestAppTokenSourceSharesOneRefreshAcrossConcurrentCallers(t *testing.T) {
	clock := &fakeClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	fake, server := newFakeGitHub(t, testKey(t), clock)
	source := newTestSource(t, server.URL, clock, nil)

	var wg sync.WaitGroup
	tokens := make([]string, 25)
	for i := range tokens {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := source.BearerToken(context.Background())
			if err != nil {
				t.Errorf("BearerToken() error = %v", err)
			}
			tokens[i] = token
		}()
	}
	wg.Wait()
	for _, token := range tokens {
		if token != tokens[0] {
			t.Fatalf("callers received different tokens: %v", tokens)
		}
	}
	if got := fake.mints.Load(); got != 1 {
		t.Fatalf("mint calls = %d, want 1 shared refresh", got)
	}
}

func TestAppTokenSourceFallsBackToValidCachedTokenOnRefreshFailure(t *testing.T) {
	clock := &fakeClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	fake, server := newFakeGitHub(t, testKey(t), clock)
	source := newTestSource(t, server.URL, clock, nil)

	first, err := source.BearerToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fake.status.Store(http.StatusInternalServerError)
	clock.Advance(56 * time.Minute) // refresh is due, but the token is valid for 4 more minutes
	got, err := source.BearerToken(context.Background())
	if err != nil || got != first {
		t.Fatalf("BearerToken() = %q, %v; want cached %q", got, err, first)
	}

	clock.Advance(10 * time.Minute) // now expired: the failure must surface
	if _, err := source.BearerToken(context.Background()); err == nil {
		t.Fatal("expired token with a failing refresh unexpectedly succeeded")
	}
}

func TestAppTokenSourceErrorsDoNotLeakCredentials(t *testing.T) {
	clock := &fakeClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	fake, server := newFakeGitHub(t, testKey(t), clock)
	fake.status.Store(http.StatusUnauthorized)
	source := newTestSource(t, server.URL, clock, nil)

	_, err := source.BearerToken(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("error = %v, want APIError 401", err)
	}
	message := err.Error()
	for _, forbidden := range []string{"BEGIN", "eyJ", string(pkcs1PEM(testKey(t)))[30:60]} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("error message leaks credential material (%q): %s", forbidden, message)
		}
	}
}

func TestAppTokenSourceNarrowsPermissions(t *testing.T) {
	clock := &fakeClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	fake, server := newFakeGitHub(t, testKey(t), clock)
	source := newTestSource(t, server.URL, clock, func(c *AppTokenSourceConfig) {
		c.Permissions = map[string]string{"administration": "write"}
	})
	if _, err := source.BearerToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if body, _ := fake.lastBody.Load().(string); body != `{"permissions":{"administration":"write"}}` {
		t.Fatalf("request body = %q", body)
	}
}

func TestAppTokenSourceRejectsUnusableResponses(t *testing.T) {
	tests := map[string]string{
		"empty token":       `{"token":"","expires_at":"2099-01-01T00:00:00Z"}`,
		"whitespace token":  `{"token":"a b","expires_at":"2099-01-01T00:00:00Z"}`,
		"missing expiry":    `{"token":"ghs_ok"}`,
		"already expired":   `{"token":"ghs_ok","expires_at":"2001-01-01T00:00:00Z"}`,
		"not json":          `<html>`,
		"unknown structure": `[]`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			clock := &fakeClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
			source := newTestSource(t, server.URL, clock, nil)
			if token, err := source.BearerToken(context.Background()); err == nil {
				t.Fatalf("BearerToken() = %q, want error", token)
			}
		})
	}
}

func TestAppTokenSourceHonorsCallerContext(t *testing.T) {
	clock := &fakeClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	defer close(release)
	source := newTestSource(t, server.URL, clock, nil)

	// The first caller starts a slow refresh; a second caller with a short
	// deadline must give up without waiting for it.
	go func() { _, _ = source.BearerToken(context.Background()) }()
	deadline := time.Now().Add(2 * time.Second)
	for source.refreshInflight() == false && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := source.BearerToken(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	cancelled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if _, err := source.BearerToken(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want canceled", err)
	}
}

func (s *AppTokenSource) refreshInflight() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inflight != nil
}

func TestParseAppPrivateKeyFormats(t *testing.T) {
	key := testKey(t)
	p256, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	escaped := strings.ReplaceAll(strings.TrimSpace(string(pkcs1PEM(key))), "\n", `\n`)

	valid := map[string][]byte{
		"pkcs1":                 pkcs1PEM(key),
		"pkcs8":                 pkcs8PEM(t, key),
		"surrounding space":     []byte("\n  " + string(pkcs1PEM(key)) + "\n"),
		"escaped newlines":      []byte(escaped),
		"pkcs1 with CRLF lines": []byte(strings.ReplaceAll(string(pkcs1PEM(key)), "\n", "\r\n")),
	}
	for name, data := range valid {
		if _, err := parseAppPrivateKey(data); err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
		}
	}

	invalid := map[string][]byte{
		"empty":         nil,
		"garbage":       []byte("not a key"),
		"ec key":        pkcs8PEM(t, p256),
		"too small":     pkcs1PEM(small),
		"public key":    pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("x")}),
		"truncated pem": []byte(strings.TrimSpace(string(pkcs1PEM(key)))[:200]),
		"encrypted pem": pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Headers: map[string]string{"Proc-Type": "4,ENCRYPTED", "DEK-Info": "AES-256-CBC,00"}, Bytes: []byte("x")}),
	}
	for name, data := range invalid {
		_, err := parseAppPrivateKey(data)
		if err == nil {
			t.Errorf("%s: expected an error", name)
			continue
		}
		if strings.Contains(err.Error(), "MII") || strings.Contains(err.Error(), "BEGIN") {
			t.Errorf("%s: error leaks key material: %v", name, err)
		}
	}
}

func TestNewAppTokenSourceValidatesConfig(t *testing.T) {
	key := pkcs1PEM(testKey(t))
	tests := map[string]AppTokenSourceConfig{
		"missing app id":       {InstallationID: 1, PrivateKeyPEM: key},
		"bad app id":           {AppID: "a b", InstallationID: 1, PrivateKeyPEM: key},
		"missing installation": {AppID: "1", PrivateKeyPEM: key},
		"missing key":          {AppID: "1", InstallationID: 1},
		"plain http remote":    {AppID: "1", InstallationID: 1, PrivateKeyPEM: key, BaseURL: "http://github.example.com"},
		"url with credentials": {AppID: "1", InstallationID: 1, PrivateKeyPEM: key, BaseURL: "https://user:pass@github.example.com"},
		"file scheme":          {AppID: "1", InstallationID: 1, PrivateKeyPEM: key, BaseURL: "file:///etc/passwd"},
		"bad permission level": {AppID: "1", InstallationID: 1, PrivateKeyPEM: key, Permissions: map[string]string{"administration": "owner"}},
		"bad permission name":  {AppID: "1", InstallationID: 1, PrivateKeyPEM: key, Permissions: map[string]string{"Admin Rights": "read"}},
	}
	for name, config := range tests {
		if _, err := NewAppTokenSource(config); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	for _, base := range []string{"", "https://api.github.com", "https://ghes.example.com/api/v3", "http://127.0.0.1:8080", "http://localhost:9"} {
		if _, err := NewAppTokenSource(AppTokenSourceConfig{AppID: "1", InstallationID: 1, PrivateKeyPEM: key, BaseURL: base}); err != nil {
			t.Errorf("base URL %q: unexpected error %v", base, err)
		}
	}
}

func TestAppTokenSourceKeepsEnterprisePathPrefix(t *testing.T) {
	var path atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path.Store(r.URL.Path)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"token":"ghs_ok","expires_at":"2099-01-01T00:00:00Z"}`)
	}))
	defer server.Close()
	clock := &fakeClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	source := newTestSource(t, server.URL+"/api/v3", clock, nil)
	if _, err := source.BearerToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, _ := path.Load().(string); got != "/api/v3/app/installations/4242/access_tokens" {
		t.Fatalf("path = %q", got)
	}
}

func TestPermissionsString(t *testing.T) {
	if s := PermissionsString(map[string]string{"b": "read", "a": "write"}); s != "a=write,b=read" {
		t.Fatalf("PermissionsString() = %q", s)
	}
	if err := ValidatePermissions(map[string]string{"administration": "write"}); err != nil {
		t.Fatal(err)
	}
}

// TestJITClientUsesAppTokenSource proves the source plugs into the existing
// client unchanged: one token mint serves several JIT requests, and the JIT
// request carries the installation token, not the App JWT.
func TestJITClientUsesAppTokenSource(t *testing.T) {
	clock := &fakeClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	fake, server := newFakeGitHubWithJIT(t, clock)
	source := newTestSource(t, server.URL, clock, nil)
	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: source, Retry: RetryConfig{MaxAttempts: 1}})
	if err != nil {
		t.Fatal(err)
	}
	group := int64(7)
	for i := 0; i < 3; i++ {
		response, err := client.GenerateJITConfig(context.Background(), "acme", "widgets", JITConfigRequest{Name: "r", RunnerGroupID: &group, Labels: []string{"self-hosted"}})
		if err != nil || response.EncodedJITConfig != "encoded" {
			t.Fatalf("GenerateJITConfig() = %+v, %v", response, err)
		}
	}
	if got := fake.mints.Load(); got != 1 {
		t.Fatalf("token mints = %d, want 1", got)
	}
}

func newFakeGitHubWithJIT(t *testing.T, clock *fakeClock) (*fakeGitHub, *httptest.Server) {
	t.Helper()
	f := &fakeGitHub{t: t, pub: &testKey(t).PublicKey, clock: clock, lifetime: time.Hour}
	f.status.Store(http.StatusCreated)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/generate-jitconfig") {
			if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ghs_installation_") {
				t.Errorf("JIT request authorization = %q, want an installation token", got)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"runner":{"id":1,"name":"r"},"encoded_jit_config":"encoded"}`)
			return
		}
		f.serve(w, r)
	}))
	t.Cleanup(server.Close)
	return f, server
}
