// Package archive provides a small durable, redacted JSONL event archive.
package archive

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/telemetry"
)

const (
	defaultMaxLineBytes = 1 << 20
	archiveMode         = 0o600
	redacted            = "[REDACTED]"
	maxRetentionFiles   = 64
)

var (
	ErrInvalidEvent  = errors.New("invalid archive event")
	ErrLineTooLarge  = errors.New("archive event exceeds maximum line size")
	ErrClosed        = errors.New("event archive is closed")
	ErrInvalidPolicy = errors.New("invalid archive retention policy")
)

// Emit adapts the existing lifecycle telemetry sink to the durable archive.
// Metadata is redacted by Append before it is written.
func (a *Archive) Emit(event telemetry.Event) error {
	return a.Append(context.Background(), Event{
		ID: event.ID, Type: event.Type, Timestamp: event.At,
		JobID: event.JobID, RunnerID: event.RunnerID, Provider: event.Provider,
		Correlation: map[string]string{"job_id": event.JobID, "runner_id": event.RunnerID},
		Data:        map[string]any{"metadata": event.Metadata},
	})
}

var _ telemetry.Sink = (*Archive)(nil)

// Event is the JSONL contract stored by the archive. Data is intentionally
// untyped at the boundary so the archive can retain bounded diagnostics
// without depending on a controller package.
type Event struct {
	ID          string            `json:"id,omitempty"`
	Type        string            `json:"type"`
	Timestamp   time.Time         `json:"timestamp"`
	JobID       string            `json:"job_id,omitempty"`
	RunnerID    string            `json:"runner_id,omitempty"`
	LeaseID     string            `json:"lease_id,omitempty"`
	Provider    string            `json:"provider,omitempty"`
	Correlation map[string]string `json:"correlation,omitempty"`
	Data        map[string]any    `json:"data,omitempty"`
}

// Filter selects replay records. Since is inclusive; Until is exclusive.
type Filter struct {
	Types []string
	Since time.Time
	Until time.Time
}

// Options controls archive creation and redaction.
type Options struct {
	MaxLineBytes   int
	RedactedValues []string
	Retention      RetentionPolicy
}

// RetentionPolicy bounds the active JSONL file and its rotated siblings.
// MaxFiles includes the active file. Zero values disable rotation.
type RetentionPolicy struct {
	MaxBytes int64
	MaxFiles int
	MaxAge   time.Duration
}

func (p RetentionPolicy) Validate() error {
	if p.MaxBytes == 0 && p.MaxFiles == 0 && p.MaxAge == 0 {
		return nil
	}
	if p.MaxBytes != 0 && (p.MaxBytes < 1<<20 || p.MaxBytes > 1<<30) {
		return fmt.Errorf("%w: max bytes must be 0 or between 1 MiB and 1 GiB", ErrInvalidPolicy)
	}
	if p.MaxFiles != 0 && (p.MaxFiles < 2 || p.MaxFiles > maxRetentionFiles) {
		return fmt.Errorf("%w: max files must be 0 or between 2 and %d", ErrInvalidPolicy, maxRetentionFiles)
	}
	if p.MaxAge != 0 && (p.MaxAge < time.Hour || p.MaxAge > 30*24*time.Hour) {
		return fmt.Errorf("%w: max age must be 0 or between 1 hour and 30 days", ErrInvalidPolicy)
	}
	if (p.MaxBytes != 0 || p.MaxAge != 0) && p.MaxFiles == 0 {
		return fmt.Errorf("%w: max files is required when rotation is enabled", ErrInvalidPolicy)
	}
	if p.MaxBytes == 0 && p.MaxAge == 0 && p.MaxFiles != 0 {
		return fmt.Errorf("%w: max files requires a rotation trigger", ErrInvalidPolicy)
	}
	return nil
}

// Archive owns one append-only archive file. A single process may safely use
// one Archive concurrently; separate processes should use a filesystem lock or
// a single archive owner because O_APPEND does not make flushes transactional.
type Archive struct {
	path      string
	file      *os.File
	max       int
	secrets   []string
	retention RetentionPolicy
	mu        sync.RWMutex
	closed    bool
}

// Open creates or opens path without truncating it. The archive and all
// existing content are restricted to owner read/write permissions.
func Open(path string, options Options) (*Archive, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%w: archive path is required", ErrInvalidEvent)
	}
	if options.MaxLineBytes <= 0 {
		options.MaxLineBytes = defaultMaxLineBytes
	}
	if options.MaxLineBytes < 32 {
		return nil, fmt.Errorf("%w: maximum line size is too small", ErrLineTooLarge)
	}
	if err := options.Retention.Validate(); err != nil {
		return nil, err
	}
	if err := ensureArchivePath(path); err != nil {
		return nil, err
	}
	staleRotated, err := rotateStale(path, options.Retention, time.Now())
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, archiveMode)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	if err := f.Chmod(archiveMode); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("restrict archive permissions: %w", err)
	}
	archive := &Archive{path: path, file: f, max: options.MaxLineBytes, secrets: cleanSecrets(options.RedactedValues), retention: options.Retention}
	if err := pruneRotated(path, options.Retention, time.Now(), staleRotated); err != nil {
		_ = f.Close()
		return nil, err
	}
	return archive, nil
}

func ensureArchivePath(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("archive path is not a regular file")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect archive: %w", err)
	}
	if parent := filepath.Dir(path); parent != "." {
		if info, err := os.Stat(parent); err != nil || !info.IsDir() {
			if err == nil {
				err = errors.New("parent is not a directory")
			}
			return fmt.Errorf("archive parent: %w", err)
		}
	}
	return nil
}

// Append redacts and durably appends one JSON object followed by a newline.
func (a *Archive) Append(ctx context.Context, event Event) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(event.Type) == "" || event.Timestamp.IsZero() {
		return fmt.Errorf("%w: type and timestamp are required", ErrInvalidEvent)
	}
	if event.Timestamp.Location() != time.UTC {
		event.Timestamp = event.Timestamp.UTC()
	}
	encoded, err := json.Marshal(redactEvent(event, a.secrets))
	if err != nil {
		return fmt.Errorf("marshal archive event: %w", err)
	}
	if len(encoded)+1 > a.max {
		return fmt.Errorf("%w: %d bytes exceeds %d", ErrLineTooLarge, len(encoded)+1, a.max)
	}
	encoded = append(encoded, '\n')

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return ErrClosed
	}
	if err := contextErr(ctx); err != nil {
		return err
	}
	if err := a.rotateIfNeeded(len(encoded), time.Now()); err != nil {
		return err
	}
	for written := 0; written < len(encoded); {
		n, err := a.file.Write(encoded[written:])
		written += n
		if err != nil {
			return fmt.Errorf("append archive event: %w", err)
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	if err := a.file.Sync(); err != nil {
		return fmt.Errorf("flush archive event: %w", err)
	}
	return nil
}

func (a *Archive) rotateIfNeeded(incoming int, now time.Time) error {
	if a.retention.MaxBytes == 0 && a.retention.MaxAge == 0 {
		return nil
	}
	info, err := a.file.Stat()
	if err != nil {
		return fmt.Errorf("stat archive: %w", err)
	}
	rotate := a.retention.MaxBytes > 0 && info.Size()+int64(incoming) > a.retention.MaxBytes
	rotate = rotate || (a.retention.MaxAge > 0 && info.Size() > 0 && now.Sub(info.ModTime()) >= a.retention.MaxAge)
	if !rotate {
		return nil
	}
	if err := a.file.Close(); err != nil {
		return fmt.Errorf("close archive for rotation: %w", err)
	}
	if err := rotatePath(a.path, a.retention); err != nil {
		return err
	}
	f, err := os.OpenFile(a.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, archiveMode)
	if err != nil {
		return fmt.Errorf("reopen archive after rotation: %w", err)
	}
	if err := f.Chmod(archiveMode); err != nil {
		_ = f.Close()
		return fmt.Errorf("restrict rotated archive permissions: %w", err)
	}
	a.file = f
	return pruneRotated(a.path, a.retention, now, true)
}

func rotateStale(path string, policy RetentionPolicy, now time.Time) (bool, error) {
	if policy.MaxAge == 0 {
		return false, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat archive for rotation: %w", err)
	}
	if info.Size() == 0 || now.Sub(info.ModTime()) < policy.MaxAge {
		return false, nil
	}
	return true, rotatePath(path, policy)
}

func rotatePath(path string, policy RetentionPolicy) error {
	for index := policy.MaxFiles - 1; index >= 1; index-- {
		source := fmt.Sprintf("%s.%d", path, index)
		target := fmt.Sprintf("%s.%d", path, index+1)
		if index+1 >= policy.MaxFiles {
			if err := os.Remove(source); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove expired archive: %w", err)
			}
			continue
		}
		if err := os.Rename(source, target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("rotate archive: %w", err)
		}
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return fmt.Errorf("rotate archive: %w", err)
	}
	return os.Chmod(path+".1", archiveMode)
}

func pruneRotated(path string, policy RetentionPolicy, now time.Time, preserveNewest bool) error {
	if policy.MaxFiles >= 2 {
		for index := policy.MaxFiles; index >= 2; index-- {
			if err := os.Remove(fmt.Sprintf("%s.%d", path, index)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("prune archive: %w", err)
			}
		}
	}
	if policy.MaxAge == 0 {
		return nil
	}
	start := 1
	if preserveNewest {
		// A just-rotated file can legitimately predate the age threshold.
		start = 2
	}
	for index := start; index < policy.MaxFiles; index++ {
		rotated := fmt.Sprintf("%s.%d", path, index)
		info, err := os.Stat(rotated)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect rotated archive: %w", err)
		}
		if now.Sub(info.ModTime()) >= policy.MaxAge {
			if err := os.Remove(rotated); err != nil {
				return fmt.Errorf("remove aged archive: %w", err)
			}
		}
	}
	return nil
}

// Replay reads a consistent snapshot of the archive and applies filter.
func (a *Archive) Replay(ctx context.Context, filter Filter) ([]Event, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	a.mu.RLock()
	if a.closed {
		a.mu.RUnlock()
		return nil, ErrClosed
	}
	path := a.path
	max := a.max
	a.mu.RUnlock()
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	defer f.Close()
	types := make(map[string]struct{}, len(filter.Types))
	for _, typ := range filter.Types {
		if strings.TrimSpace(typ) != "" {
			types[typ] = struct{}{}
		}
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), max)
	result := make([]Event, 0)
	for scanner.Scan() {
		if err := contextErr(ctx); err != nil {
			return nil, err
		}
		line := scanner.Bytes()
		if len(line)+1 > max {
			return nil, ErrLineTooLarge
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("decode archive event: %w", err)
		}
		if strings.TrimSpace(event.Type) == "" || event.Timestamp.IsZero() {
			return nil, fmt.Errorf("%w: archived record is missing type or timestamp", ErrInvalidEvent)
		}
		if len(types) > 0 {
			if _, ok := types[event.Type]; !ok {
				continue
			}
		}
		if !filter.Since.IsZero() && event.Timestamp.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && !event.Timestamp.Before(filter.Until) {
			continue
		}
		result = append(result, event)
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, ErrLineTooLarge
		}
		return nil, fmt.Errorf("scan archive: %w", err)
	}
	return result, nil
}

// Close flushes and closes the archive. It is safe to call more than once.
func (a *Archive) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	if err := a.file.Sync(); err != nil {
		_ = a.file.Close()
		return err
	}
	return a.file.Close()
}

// FilterEvents applies the same filtering semantics to an already replayed
// slice and returns a new slice, preserving archive order.
func FilterEvents(events []Event, filter Filter) []Event {
	types := make(map[string]struct{}, len(filter.Types))
	for _, typ := range filter.Types {
		types[typ] = struct{}{}
	}
	result := make([]Event, 0, len(events))
	for _, event := range events {
		if len(types) > 0 {
			if _, ok := types[event.Type]; !ok {
				continue
			}
		}
		if !filter.Since.IsZero() && event.Timestamp.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && !event.Timestamp.Before(filter.Until) {
			continue
		}
		result = append(result, event)
	}
	return result
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func cleanSecrets(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return len(result[i]) > len(result[j]) })
	return result
}

var secretPattern = regexp.MustCompile(`(?i)(gh[pousr]_[A-Za-z0-9_\-]{10,}|github_pat_[A-Za-z0-9_\-]{10,}|AKIA[0-9A-Z]{16}|Bearer\s+[A-Za-z0-9._\-+/=]{10,})`)

func redactEvent(event Event, secrets []string) Event {
	event.Correlation = redactStringMap(event.Correlation, secrets)
	event.Data = redactMap(event.Data, secrets)
	return event
}

func redactStringMap(input map[string]string, secrets []string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		if sensitiveKey(key) {
			output[key] = redacted
		} else {
			output[key] = redactString(value, secrets)
		}
	}
	return output
}

func redactMap(input map[string]any, secrets []string) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		if sensitiveKey(key) {
			output[key] = redacted
		} else {
			output[key] = redactValue(value, secrets)
		}
	}
	return output
}

func redactValue(value any, secrets []string) any {
	switch value := value.(type) {
	case map[string]any:
		return redactMap(value, secrets)
	case map[string]string:
		return redactStringMap(value, secrets)
	case []any:
		output := make([]any, len(value))
		for i, item := range value {
			output[i] = redactValue(item, secrets)
		}
		return output
	case string:
		return redactString(value, secrets)
	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	for _, part := range []string{"token", "secret", "password", "credential", "private_key", "jit_config", "authorization"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func redactString(value string, secrets []string) string {
	for _, secret := range secrets {
		if subtle.ConstantTimeCompare([]byte(value), []byte(secret)) == 1 {
			return redacted
		}
		value = strings.ReplaceAll(value, secret, redacted)
	}
	return secretPattern.ReplaceAllString(value, redacted)
}
