package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// appJWTBackdate allows for clock drift between this host and GitHub, as
	// GitHub's documentation recommends.
	appJWTBackdate = 60 * time.Second
	// appJWTLifetime must stay below GitHub's 10 minute maximum.
	appJWTLifetime = 9 * time.Minute

	defaultAppRefreshBefore = 5 * time.Minute
	// appTokenMinValidity is how long a cached token must still be valid for
	// it to be used as a fallback when a refresh fails.
	appTokenMinValidity = 30 * time.Second

	minAppKeyBits         = 2048
	maxAppTokenResponse   = 1 << 20
	maxAppTokenPermission = 64
)

var (
	permissionNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	permissionLevels = map[string]struct{}{"read": {}, "write": {}, "admin": {}}
	// A numeric App ID or a client ID are the identifiers GitHub accepts as the
	// JWT issuer. Anything else is rejected before it reaches a request.
	appIssuerRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

// AppTokenSourceConfig configures a BearerTokenSource that authenticates as a
// GitHub App installation. The private key belongs to the deployer; it must be
// read from the deployer's own secret store and is never logged or persisted.
type AppTokenSourceConfig struct {
	// AppID is the GitHub App's numeric ID or client ID (the JWT issuer).
	AppID string
	// InstallationID is the installation whose tokens are minted.
	InstallationID int64
	// PrivateKeyPEM is the App's RSA private key in PEM form (PKCS#1 or PKCS#8).
	PrivateKeyPEM []byte
	// BaseURL defaults to GitHub's public API. It must be HTTPS, except for a
	// loopback address (used by tests).
	BaseURL string
	// Permissions optionally narrows the minted token below what the App was
	// granted, for example {"administration": "write"}. Empty keeps the App's
	// full installation permissions.
	Permissions map[string]string
	// Repositories optionally restricts the minted token to these repositories
	// of the installation's account (names, not owner/name). Empty keeps every
	// repository the installation can reach. The App's grant on a repository
	// outside the list is then unusable through this token, so a defect or a
	// compromise cannot act on it.
	Repositories []string
	HTTPClient   *http.Client
	UserAgent    string
	Timeout      time.Duration
	Retry        RetryConfig
	// RefreshBefore is how long before expiry a new token is requested.
	RefreshBefore time.Duration
	// Now is injectable for tests.
	Now func() time.Time
}

// maxAppTokenRepositories is GitHub's limit for one installation token request.
const maxAppTokenRepositories = 500

var tokenRepositoryNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// ValidateRepositories checks a token-restricting repository list.
func ValidateRepositories(repositories []string) error {
	if len(repositories) > maxAppTokenRepositories {
		return fmt.Errorf("too many token repositories (max %d)", maxAppTokenRepositories)
	}
	for _, name := range repositories {
		if !tokenRepositoryNameRE.MatchString(name) || name == "." || name == ".." {
			return fmt.Errorf("invalid token repository name %q (give the repository name without its owner)", name)
		}
	}
	return nil
}

// ValidatePermissions checks a token-narrowing permission set.
func ValidatePermissions(permissions map[string]string) error {
	if len(permissions) > maxAppTokenPermission {
		return fmt.Errorf("too many token permissions (max %d)", maxAppTokenPermission)
	}
	for name, level := range permissions {
		if !permissionNameRE.MatchString(name) {
			return fmt.Errorf("invalid token permission name %q", name)
		}
		if _, ok := permissionLevels[level]; !ok {
			return fmt.Errorf("invalid level %q for token permission %q (want read, write, or admin)", level, name)
		}
	}
	return nil
}

// AppTokenSource mints and caches short-lived installation access tokens. It
// is safe for concurrent use; concurrent callers share one refresh.
type AppTokenSource struct {
	appID          string
	installationID int64
	key            *rsa.PrivateKey
	permissions    map[string]string
	repositories   []string
	client         *Client
	endpoint       string
	timeout        time.Duration
	refreshBefore  time.Duration
	now            func() time.Time

	mu        sync.Mutex
	token     string
	expiresAt time.Time
	inflight  *appRefresh
}

type appRefresh struct {
	done chan struct{}
	err  error
}

// NewAppTokenSource validates the configuration and parses the private key.
// Errors never include key material.
func NewAppTokenSource(config AppTokenSourceConfig) (*AppTokenSource, error) {
	appID := strings.TrimSpace(config.AppID)
	if !appIssuerRE.MatchString(appID) {
		return nil, errors.New("GitHub App ID must be the App's numeric ID or client ID")
	}
	if config.InstallationID <= 0 {
		return nil, errors.New("GitHub App installation ID must be positive")
	}
	if err := ValidatePermissions(config.Permissions); err != nil {
		return nil, err
	}
	key, err := parseAppPrivateKey(config.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.New("GitHub API base URL is not a valid URL")
	}
	if err := requireSecureBaseURL(parsed); err != nil {
		return nil, err
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultJITTimeout
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	userAgent := strings.TrimSpace(config.UserAgent)
	if userAgent == "" {
		userAgent = "leo-runners-controller"
	}
	refreshBefore := config.RefreshBefore
	if refreshBefore <= 0 {
		refreshBefore = defaultAppRefreshBefore
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}

	base := *parsed
	rawPath := strings.TrimRight(base.EscapedPath(), "/") + "/app/installations/" + strconv.FormatInt(config.InstallationID, 10) + "/access_tokens"
	base.RawPath = rawPath
	decoded, err := url.PathUnescape(rawPath)
	if err != nil {
		return nil, errors.New("GitHub API base URL path is invalid")
	}
	base.Path = decoded

	if err := ValidateRepositories(config.Repositories); err != nil {
		return nil, err
	}
	repositories := normalizeRepositories(config.Repositories)
	permissions := make(map[string]string, len(config.Permissions))
	for name, level := range config.Permissions {
		permissions[name] = level
	}
	return &AppTokenSource{
		appID: appID, installationID: config.InstallationID, key: key, permissions: permissions, repositories: repositories,
		client:   &Client{baseURL: parsed, httpClient: httpClient, userAgent: userAgent, timeout: timeout, retry: normalizeRetryConfig(config.Retry)},
		endpoint: base.String(), timeout: timeout, refreshBefore: refreshBefore, now: now,
	}, nil
}

func requireSecureBaseURL(u *url.URL) error {
	if u.Host == "" || u.User != nil {
		return errors.New("GitHub API base URL must be an absolute URL without credentials")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return errors.New("GitHub API base URL must use HTTPS when authenticating as a GitHub App")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// BearerToken returns a valid installation access token, refreshing it shortly
// before it expires. If a refresh fails while the cached token is still valid,
// the cached token is returned so a brief GitHub outage does not stop work.
func (s *AppTokenSource) BearerToken(ctx context.Context) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		s.mu.Lock()
		now := s.now()
		if s.token != "" && now.Before(s.expiresAt.Add(-s.refreshBefore)) {
			token := s.token
			s.mu.Unlock()
			return token, nil
		}
		if s.inflight == nil {
			call := &appRefresh{done: make(chan struct{})}
			s.inflight = call
			s.mu.Unlock()

			// The refresh is shared by every waiting caller, so it must not be
			// cancelled just because the caller that started it gave up.
			refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout*2)
			token, expiresAt, err := s.mint(refreshCtx)
			cancel()

			s.mu.Lock()
			if err == nil {
				s.token, s.expiresAt = token, expiresAt
			}
			call.err = err
			s.inflight = nil
			cached, cachedExpiry := s.token, s.expiresAt
			close(call.done)
			s.mu.Unlock()

			if err == nil {
				return token, nil
			}
			if cached != "" && s.now().Before(cachedExpiry.Add(-appTokenMinValidity)) {
				return cached, nil
			}
			return "", err
		}
		call := s.inflight
		s.mu.Unlock()

		select {
		case <-call.done:
			s.mu.Lock()
			cached, cachedExpiry := s.token, s.expiresAt
			s.mu.Unlock()
			if call.err == nil {
				return cached, nil
			}
			if cached != "" && s.now().Before(cachedExpiry.Add(-appTokenMinValidity)) {
				return cached, nil
			}
			return "", call.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

type installationTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *AppTokenSource) mint(ctx context.Context) (string, time.Time, error) {
	jwt, err := s.signJWT()
	if err != nil {
		return "", time.Time{}, err
	}
	var body []byte
	if len(s.permissions) > 0 || len(s.repositories) > 0 {
		request := map[string]any{}
		if len(s.permissions) > 0 {
			request["permissions"] = s.permissions
		}
		if len(s.repositories) > 0 {
			request["repositories"] = s.repositories
		}
		body, err = json.Marshal(request)
		if err != nil {
			return "", time.Time{}, errors.New("encode GitHub installation token request")
		}
	}
	response, err := s.client.doWithRetry(ctx, func(attemptCtx context.Context) (*http.Request, error) {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		request, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, s.endpoint, reader)
		if err != nil {
			return nil, errors.New("create GitHub installation token request")
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
		request.Header.Set("Authorization", "Bearer "+jwt)
		request.Header.Set("User-Agent", s.client.userAgent)
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		return request, nil
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("request GitHub installation token: %w", scrubTransportError(err))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("request GitHub installation token: %w", apiErrorFromResponse(response))
	}
	var parsed installationTokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, maxAppTokenResponse)).Decode(&parsed); err != nil {
		return "", time.Time{}, errors.New("decode GitHub installation token response")
	}
	token := strings.TrimSpace(parsed.Token)
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return "", time.Time{}, errors.New("GitHub installation token response has no usable token")
	}
	if parsed.ExpiresAt.IsZero() || !parsed.ExpiresAt.After(s.now().Add(appTokenMinValidity)) {
		return "", time.Time{}, errors.New("GitHub installation token response has no usable expiry")
	}
	return token, parsed.ExpiresAt, nil
}

// scrubTransportError drops URL details from transport errors so request
// metadata is not echoed into logs.
func scrubTransportError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func (s *AppTokenSource) signJWT() (string, error) {
	now := s.now()
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", errors.New("encode GitHub App JWT header")
	}
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-appJWTBackdate).Unix(),
		"exp": now.Add(appJWTLifetime).Unix(),
		"iss": s.appID,
	})
	if err != nil {
		return "", errors.New("encode GitHub App JWT claims")
	}
	signingInput := b64(header) + "." + b64(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", errors.New("sign GitHub App JWT")
	}
	return signingInput + "." + b64(signature), nil
}

func b64(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }

// parseAppPrivateKey accepts a PEM-encoded RSA key (PKCS#1 or PKCS#8). A key
// whose newlines were stored as the two characters `\n` is also accepted. The
// returned errors never contain any part of the key.
func parseAppPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil, errors.New("GitHub App private key is empty")
	}
	block, _ := pem.Decode([]byte(text))
	if block == nil && strings.Contains(text, `\n`) {
		block, _ = pem.Decode([]byte(strings.ReplaceAll(text, `\n`, "\n")))
	}
	if block == nil {
		return nil, errors.New("GitHub App private key is not valid PEM")
	}
	if _, encrypted := block.Headers["DEK-Info"]; encrypted || strings.Contains(block.Headers["Proc-Type"], "ENCRYPTED") {
		return nil, errors.New("GitHub App private key must not be passphrase-protected")
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		parsed, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.New("GitHub App private key is not a valid PKCS#1 RSA key")
		}
		key = parsed
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.New("GitHub App private key is not a valid PKCS#8 key")
		}
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("GitHub App private key must be an RSA key")
		}
		key = rsaKey
	default:
		return nil, errors.New("GitHub App private key must be an RSA private key")
	}
	if key.N.BitLen() < minAppKeyBits {
		return nil, fmt.Errorf("GitHub App private key must be at least %d bits", minAppKeyBits)
	}
	if err := key.Validate(); err != nil {
		return nil, errors.New("GitHub App private key is inconsistent")
	}
	return key, nil
}

// normalizeRepositories returns the repository names sorted and without
// duplicates, so the request body is deterministic.
func normalizeRepositories(repositories []string) []string {
	seen := make(map[string]struct{}, len(repositories))
	out := make([]string, 0, len(repositories))
	for _, name := range repositories {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// PermissionsString renders a permission set deterministically, for logs and
// diagnostics (it contains no secrets).
func PermissionsString(permissions map[string]string) string {
	names := make([]string, 0, len(permissions))
	for name := range permissions {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+permissions[name])
	}
	return strings.Join(parts, ",")
}
