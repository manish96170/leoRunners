// Package eval provides a deterministic, offline evaluation boundary for
// intelligence advisories. It consumes structured telemetry and historical
// outcomes, but never invokes a model or controls a runner.
package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/intelligence/ai"
	"github.com/leo-runners/ci-platform/controller/internal/intelligence/stats"
)

type Provenance string

const (
	ProvenanceSynthetic Provenance = "synthetic"
	ProvenanceReal      Provenance = "real"
)

var (
	ErrInvalidFixture = errors.New("invalid intelligence evaluation fixture")
	ErrRegression     = errors.New("intelligence evaluation regression")
)

// TelemetrySample is the JSON-friendly historical input to the evaluator.
// It contains aggregate-safe fields only; raw logs do not belong in a
// historical evaluation fixture.
type TelemetrySample struct {
	ID            string             `json:"id"`
	Provider      string             `json:"provider"`
	Region        string             `json:"region,omitempty"`
	Outcome       stats.Outcome      `json:"outcome"`
	CacheObserved bool               `json:"cache_observed"`
	CacheHit      bool               `json:"cache_hit"`
	TimingsMS     map[string]float64 `json:"timings_ms"`
	Resources     stats.Resources    `json:"resources"`
}

func (s TelemetrySample) toStats() (stats.Sample, error) {
	if s.ID == "" || s.Provider == "" {
		return stats.Sample{}, fmt.Errorf("%w: telemetry id and provider are required", ErrInvalidFixture)
	}
	timings := make(map[string]time.Duration, len(s.TimingsMS))
	for phase, milliseconds := range s.TimingsMS {
		if strings.TrimSpace(phase) == "" || milliseconds < 0 || milliseconds > 86_400_000 {
			return stats.Sample{}, fmt.Errorf("%w: timing %q is invalid", ErrInvalidFixture, phase)
		}
		timings[phase] = time.Duration(milliseconds * float64(time.Millisecond))
	}
	return stats.Sample{ID: s.ID, Provider: s.Provider, Region: s.Region, Outcome: s.Outcome,
		CacheObserved: s.CacheObserved, CacheHit: s.CacheHit, Timings: timings, Resources: s.Resources}, nil
}

// Case is a recorded advisory output paired with the expected policy-safe
// result. The response is data in this harness, not a provider invocation.
type Case struct {
	ID               string             `json:"id"`
	Log              string             `json:"log"`
	MaxLogBytes      int                `json:"max_log_bytes"`
	Response         ai.Response        `json:"response"`
	ExpectedAction   ai.SuggestedAction `json:"expected_action"`
	ExpectedClass    ai.Classification  `json:"expected_classification"`
	ExpectedAccepted bool               `json:"expected_accepted"`
}

type Fixture struct {
	Version    string            `json:"version"`
	DatasetID  string            `json:"dataset_id"`
	Provenance Provenance        `json:"provenance"`
	AIEnabled  bool              `json:"ai_enabled"`
	Policy     string            `json:"policy_version"`
	Telemetry  []TelemetrySample `json:"telemetry"`
	Cases      []Case            `json:"cases"`
}

type Thresholds struct {
	MinClassificationAccuracy float64 `json:"min_classification_accuracy"`
	MinActionAccuracy         float64 `json:"min_action_accuracy"`
	MinAcceptanceAccuracy     float64 `json:"min_acceptance_accuracy"`
	MaxPolicyViolations       int     `json:"max_policy_violations"`
	MaxBoundedLogBytes        int     `json:"max_bounded_log_bytes"`
}

type Report struct {
	Version                string       `json:"version"`
	DatasetID              string       `json:"dataset_id"`
	Provenance             Provenance   `json:"provenance"`
	PolicyVersion          string       `json:"policy_version"`
	AIEnabled              bool         `json:"ai_enabled"`
	TelemetryCount         int          `json:"telemetry_count"`
	CaseCount              int          `json:"case_count"`
	ClassificationAccuracy float64      `json:"classification_accuracy"`
	ActionAccuracy         float64      `json:"action_accuracy"`
	AcceptanceAccuracy     float64      `json:"acceptance_accuracy"`
	PolicyViolations       int          `json:"policy_violations"`
	RejectedCases          int          `json:"rejected_cases"`
	BoundedLogBytes        int          `json:"bounded_log_bytes"`
	HistoricalStats        stats.Report `json:"historical_stats"`
	CaseResults            []CaseResult `json:"case_results"`
	Fingerprint            string       `json:"fingerprint"`
}

type CaseResult struct {
	ID                  string             `json:"id"`
	Accepted            bool               `json:"accepted"`
	AcceptedMatch       bool               `json:"accepted_match"`
	Classification      ai.Classification  `json:"classification"`
	Action              ai.SuggestedAction `json:"action"`
	ClassificationMatch bool               `json:"classification_match"`
	ActionMatch         bool               `json:"action_match"`
	LogBytes            int                `json:"log_bytes"`
	PolicyViolation     bool               `json:"policy_violation"`
}

func Evaluate(fixture Fixture) (Report, error) {
	if err := validateFixture(fixture); err != nil {
		return Report{}, err
	}
	samples := make([]stats.Sample, 0, len(fixture.Telemetry))
	for _, sample := range fixture.Telemetry {
		converted, err := sample.toStats()
		if err != nil {
			return Report{}, err
		}
		samples = append(samples, converted)
	}
	historical, err := stats.Analyze(samples)
	if err != nil {
		return Report{}, err
	}
	report := Report{Version: "intelligence-evaluation.v1", DatasetID: fixture.DatasetID,
		Provenance: fixture.Provenance, PolicyVersion: fixture.Policy, AIEnabled: fixture.AIEnabled,
		TelemetryCount: len(fixture.Telemetry), CaseCount: len(fixture.Cases), HistoricalStats: historical}
	for _, c := range fixture.Cases {
		bounded := c.Log
		if len(bounded) > c.MaxLogBytes {
			bounded = bounded[:c.MaxLogBytes]
		}
		decision, gateErr := (ai.Gate{}).Evaluate(c.Response)
		result := CaseResult{ID: c.ID, Accepted: decision.Accepted, Classification: decision.Classification,
			LogBytes: len(bounded), PolicyViolation: errors.Is(gateErr, ai.ErrPolicyViolation)}
		result.AcceptedMatch = result.Accepted == c.ExpectedAccepted
		if len(decision.Recommendations) > 0 {
			result.Action = decision.Recommendations[0].Action
		}
		result.ClassificationMatch = result.Classification == c.ExpectedClass
		result.ActionMatch = result.Action == c.ExpectedAction
		if result.PolicyViolation {
			report.PolicyViolations++
		}
		if !result.Accepted {
			report.RejectedCases++
		}
		if result.LogBytes > report.BoundedLogBytes {
			report.BoundedLogBytes = result.LogBytes
		}
		report.CaseResults = append(report.CaseResults, result)
	}
	if len(report.CaseResults) > 0 {
		for _, result := range report.CaseResults {
			if result.ClassificationMatch {
				report.ClassificationAccuracy++
			}
			if result.ActionMatch {
				report.ActionAccuracy++
			}
			if result.AcceptedMatch {
				report.AcceptanceAccuracy++
			}
		}
		report.ClassificationAccuracy /= float64(len(report.CaseResults))
		report.ActionAccuracy /= float64(len(report.CaseResults))
		report.AcceptanceAccuracy /= float64(len(report.CaseResults))
	}
	report.Fingerprint = fingerprint(report)
	return report, nil
}

func Compare(report Report, thresholds Thresholds) error {
	if report.AIEnabled {
		return fmt.Errorf("%w: AI must remain disabled during evaluation", ErrInvalidFixture)
	}
	if report.PolicyVersion == "" || report.Provenance == "" {
		return fmt.Errorf("%w: report metadata is incomplete", ErrInvalidFixture)
	}
	if report.ClassificationAccuracy < thresholds.MinClassificationAccuracy || report.ActionAccuracy < thresholds.MinActionAccuracy || report.AcceptanceAccuracy < thresholds.MinAcceptanceAccuracy {
		return fmt.Errorf("%w: accuracy below threshold", ErrRegression)
	}
	if report.PolicyViolations > thresholds.MaxPolicyViolations {
		return fmt.Errorf("%w: policy violations exceed threshold", ErrRegression)
	}
	if report.BoundedLogBytes > thresholds.MaxBoundedLogBytes {
		return fmt.Errorf("%w: bounded log exceeds threshold", ErrRegression)
	}
	return nil
}

func validateFixture(f Fixture) error {
	if f.Version != "intelligence-evaluation.v1" || strings.TrimSpace(f.DatasetID) == "" || strings.TrimSpace(f.Policy) == "" {
		return fmt.Errorf("%w: version, dataset_id, and policy_version are required", ErrInvalidFixture)
	}
	if f.AIEnabled {
		return fmt.Errorf("%w: AI must be disabled", ErrInvalidFixture)
	}
	if f.Provenance != ProvenanceSynthetic && f.Provenance != ProvenanceReal {
		return fmt.Errorf("%w: provenance must be synthetic or real", ErrInvalidFixture)
	}
	if len(f.Telemetry) == 0 || len(f.Telemetry) > 10000 || len(f.Cases) == 0 || len(f.Cases) > 1000 {
		return fmt.Errorf("%w: bounded telemetry and case counts are required", ErrInvalidFixture)
	}
	seen := map[string]bool{}
	for _, c := range f.Cases {
		if c.ID == "" || seen[c.ID] || c.MaxLogBytes <= 0 || c.MaxLogBytes > 262144 || len(c.Log) > 1048576 {
			return fmt.Errorf("%w: case identity or log bounds are invalid", ErrInvalidFixture)
		}
		if credentialPattern.MatchString(c.Log) {
			return fmt.Errorf("%w: credential-shaped log content is forbidden", ErrInvalidFixture)
		}
		seen[c.ID] = true
	}
	return nil
}

var credentialPattern = regexp.MustCompile(`(?i)Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|(?:password|secret|access[_-]?key)\s*[:=]\s*\S+`)

// Load decodes one bounded JSON evaluation fixture without contacting a
// provider. Unknown fields are rejected so policy changes are reviewable.
func Load(r io.Reader) (Fixture, error) {
	decoder := json.NewDecoder(io.LimitReader(r, 8<<20))
	decoder.DisallowUnknownFields()
	var fixture Fixture
	if err := decoder.Decode(&fixture); err != nil {
		return Fixture{}, fmt.Errorf("%w: %v", ErrInvalidFixture, err)
	}
	return fixture, nil
}

// EvaluateJSON is the file-oriented entry point used by offline tooling.
func EvaluateJSON(r io.Reader) (Report, error) {
	fixture, err := Load(r)
	if err != nil {
		return Report{}, err
	}
	return Evaluate(fixture)
}

func fingerprint(report Report) string {
	ids := make([]string, 0, len(report.CaseResults))
	for _, result := range report.CaseResults {
		ids = append(ids, fmt.Sprintf("%s:%t:%t:%t", result.ID, result.Accepted, result.ClassificationMatch, result.ActionMatch))
	}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, "|")))
	return hex.EncodeToString(sum[:])
}
