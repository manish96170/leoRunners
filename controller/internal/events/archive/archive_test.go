package archive

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAppendRedactsAndReplaysAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	secret := "super-secret-token"
	a, err := Open(path, Options{RedactedValues: []string{secret}})
	if err != nil {
		t.Fatal(err)
	}
	first := Event{
		ID: "evt-1", Type: "runner.ready", Timestamp: time.Date(2026, 9, 13, 1, 2, 3, 0, time.FixedZone("IST", 19800)),
		Correlation: map[string]string{"job_id": "job-1", "authorization": "Bearer " + secret},
		Data:        map[string]any{"message": "token=" + secret, "nested": map[string]any{"password": "pw"}},
	}
	if err := a.Append(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), secret) || strings.Contains(string(contents), "pw") {
		t.Fatalf("archive contains a secret: %s", contents)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(contents))), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["timestamp"] != "2026-09-12T19:32:03Z" {
		t.Fatalf("timestamp was not normalized to UTC: %v", raw["timestamp"])
	}

	restarted, err := Open(path, Options{RedactedValues: []string{secret}})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err := restarted.Append(context.Background(), Event{ID: "evt-2", Type: "job.completed", Timestamp: time.Date(2026, 9, 13, 2, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	events, err := restarted.Replay(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Type != "runner.ready" || events[1].Type != "job.completed" {
		t.Fatalf("unexpected replay: %#v", events)
	}
}

func TestReplayFiltersTypeAndTime(t *testing.T) {
	a := openTestArchive(t, Options{})
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, typ := range []string{"job.queued", "runner.ready", "job.completed"} {
		if err := a.Append(context.Background(), Event{ID: string(rune('a' + i)), Type: typ, Timestamp: base.Add(time.Duration(i) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := a.Replay(context.Background(), Filter{Types: []string{"runner.ready", "job.completed"}, Since: base.Add(time.Hour), Until: base.Add(3 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Type != "runner.ready" || events[1].Type != "job.completed" {
		t.Fatalf("unexpected filtered replay: %#v", events)
	}
}

func TestAppendBoundsLineBeforeWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	a, err := Open(path, Options{MaxLineBytes: 128})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	err = a.Append(context.Background(), Event{Type: "large", Timestamp: time.Now(), Data: map[string]any{"value": strings.Repeat("x", 512)}})
	if !errors.Is(err, ErrLineTooLarge) {
		t.Fatalf("expected line-size error, got %v", err)
	}
	contents, _ := os.ReadFile(path)
	if len(contents) != 0 {
		t.Fatalf("oversized event was written: %d bytes", len(contents))
	}
}

func TestArchiveIsOwnerOnlyAndRejectsUnsafePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	a, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("archive permissions = %o, want 600", info.Mode().Perm())
	}
	if _, err := Open(t.TempDir(), Options{}); err == nil {
		t.Fatal("expected directory path to be rejected")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link, Options{}); err == nil {
		t.Fatal("expected symlink path to be rejected")
	}
}

func TestConcurrentAppendsRemainReplayable(t *testing.T) {
	a := openTestArchive(t, Options{})
	const count = 32
	var group sync.WaitGroup
	for i := 0; i < count; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := a.Append(context.Background(), Event{ID: string(rune(i + 1)), Type: "test", Timestamp: time.Now().UTC()}); err != nil {
				t.Errorf("append: %v", err)
			}
		}()
	}
	group.Wait()
	events, err := a.Replay(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != count {
		t.Fatalf("replayed %d events, want %d", len(events), count)
	}
}

func TestRetentionRotatesBeforeByteBoundAndKeepsOwnerOnlyFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	a, err := Open(path, Options{Retention: RetentionPolicy{MaxBytes: 1 << 20, MaxFiles: 3}})
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("x", 700*1024)
	for i := 0; i < 2; i++ {
		if err := a.Append(context.Background(), Event{ID: string(rune('a' + i)), Type: "large", Timestamp: time.Now().UTC(), Data: map[string]any{"value": large}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	rotated, err := os.Stat(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Mode().Perm() != 0o600 {
		t.Fatalf("rotated archive permissions = %o, want 600", rotated.Mode().Perm())
	}
	active, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(active), `"id":"b"`) {
		t.Fatalf("active archive does not contain second event: %v", err)
	}
	if _, err := os.Stat(path + ".2"); !os.IsNotExist(err) {
		t.Fatalf("unexpected second rotated file: %v", err)
	}
}

func TestRetentionRotatesStaleArchiveOnRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"id":"old","type":"old","timestamp":"2026-01-01T00:00:00Z"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	a, err := Open(path, Options{Retention: RetentionPolicy{MaxAge: time.Hour, MaxFiles: 3}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.Append(context.Background(), Event{ID: "new", Type: "new", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	oldContents, err := os.ReadFile(path + ".1")
	if err != nil || !strings.Contains(string(oldContents), `"id":"old"`) {
		t.Fatalf("stale archive was not retained after restart: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("active archive permissions: %v", err)
	}
}

func TestRetentionPolicyValidation(t *testing.T) {
	valid := []RetentionPolicy{{}, {MaxBytes: 1 << 20, MaxFiles: 2}, {MaxAge: 24 * time.Hour, MaxFiles: 4}}
	for _, policy := range valid {
		if err := policy.Validate(); err != nil {
			t.Errorf("valid policy rejected: %v", err)
		}
	}
	invalid := []RetentionPolicy{{MaxBytes: 1024, MaxFiles: 2}, {MaxBytes: 1 << 20}, {MaxFiles: 2}, {MaxAge: 31 * 24 * time.Hour, MaxFiles: 2}}
	for _, policy := range invalid {
		if err := policy.Validate(); !errors.Is(err, ErrInvalidPolicy) {
			t.Errorf("invalid policy accepted: %#v", policy)
		}
	}
}

func TestReplayRejectsMalformedAndHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.Replay(context.Background(), Filter{}); err == nil {
		t.Fatal("expected malformed record error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Replay(ctx, Filter{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func openTestArchive(t *testing.T, options Options) *Archive {
	t.Helper()
	a, err := Open(filepath.Join(t.TempDir(), "events.jsonl"), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}
