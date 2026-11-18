package observability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
)

const redacted = "[REDACTED]"

type LoggerOptions struct{ RedactedValues []string }

type JSONLogger struct {
	mu      sync.Mutex
	w       io.Writer
	secrets []string
}

var _ EventSink = (*JSONLogger)(nil)

func NewJSONLogger(w io.Writer, options LoggerOptions) *JSONLogger {
	return &JSONLogger{w: w, secrets: append([]string(nil), options.RedactedValues...)}
}

func (l *JSONLogger) Log(event LogEvent) error {
	if l == nil || l.w == nil {
		return fmt.Errorf("logger writer is nil")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Level == "" {
		event.Level = LevelInfo
	}
	if strings.TrimSpace(event.Message) == "" {
		return fmt.Errorf("log message is required")
	}
	// Marshal first so redaction covers struct fields (including the message
	// and correlation envelope) as well as arbitrary nested field values.
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	encoded, err = RedactJSON(encoded, l.secrets)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = l.w.Write(encoded)
	return err
}

func (l *JSONLogger) Lifecycle(event LifecycleEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}
	return l.Log(LogEvent{Timestamp: event.Timestamp, Level: LevelInfo, Message: event.EventType, EventType: event.EventType, Correlation: event.Correlation, Fields: map[string]any{
		"event_id": event.EventID, "from_state": event.FromState, "to_state": event.ToState, "outcome": event.Outcome, "error_class": event.ErrorClass, "duration": event.Duration, "attributes": event.Attributes,
	}})
}

func (l *JSONLogger) EmitLog(event LogEvent) error { return l.Log(event) }

func (l *JSONLogger) EmitLifecycle(event LifecycleEvent) error { return l.Lifecycle(event) }

func (l *JSONLogger) Debug(message string, correlation Correlation, fields map[string]any) error {
	return l.Log(LogEvent{Level: LevelDebug, Message: message, Correlation: correlation, Fields: fields})
}
func (l *JSONLogger) Info(message string, correlation Correlation, fields map[string]any) error {
	return l.Log(LogEvent{Level: LevelInfo, Message: message, Correlation: correlation, Fields: fields})
}
func (l *JSONLogger) Warn(message string, correlation Correlation, fields map[string]any) error {
	return l.Log(LogEvent{Level: LevelWarn, Message: message, Correlation: correlation, Fields: fields})
}
func (l *JSONLogger) Error(message string, correlation Correlation, fields map[string]any) error {
	return l.Log(LogEvent{Level: LevelError, Message: message, Correlation: correlation, Fields: fields})
}

var secretPattern = regexp.MustCompile(`(?i)(bearer\s+[^\s,;]+|(?:gh[ps]_[A-Za-z0-9_]+)|(?:AKIA[0-9A-Z]{16})|(?:xox[baprs]-[A-Za-z0-9-]+))`)

func redactValue(value any, secrets []string, key string) any {
	if isSecretKey(key) {
		return redacted
	}
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = redactValue(item, secrets, k)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(v))
		for k, item := range v {
			if isSecretKey(k) {
				out[k] = redacted
			} else {
				out[k] = redactString(item, secrets)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactValue(item, secrets, "")
		}
		return out
	case []string:
		out := make([]string, len(v))
		for i, item := range v {
			out[i] = redactString(item, secrets)
		}
		return out
	case string:
		return redactString(v, secrets)
	default:
		return value
	}
}

func redactString(value string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, redacted)
		}
	}
	return secretPattern.ReplaceAllString(value, redacted)
}

func isSecretKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	for _, part := range []string{"secret", "token", "password", "passwd", "api_key", "apikey", "private_key", "jit_config", "authorization", "credential", "user_data"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

// RedactJSON is useful to adapters that must sanitize an already structured
// payload before forwarding it elsewhere.
func RedactJSON(data []byte, secrets []string) ([]byte, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(redactValue(value, secrets, ""))
	if err != nil {
		return nil, err
	}
	return bytes.TrimSpace(encoded), nil
}
