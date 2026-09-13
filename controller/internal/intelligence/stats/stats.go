// Package stats computes deterministic historical metrics from structured
// lifecycle observations. It deliberately has no model, network, or clock
// dependency so the same samples always produce the same report.
package stats

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

var (
	ErrNoSamples       = errors.New("at least one sample is required")
	ErrInvalidSample   = errors.New("invalid sample")
	ErrDuplicateSample = errors.New("duplicate sample id")
)

// Outcome is the terminal result of a job attempt.
type Outcome string

const (
	OutcomeSuccess   Outcome = "success"
	OutcomeFailure   Outcome = "failure"
	OutcomeCancelled Outcome = "cancelled"
)

// Sample is one complete, structured observation. Timings are durations of
// named lifecycle or job phases; they are not wall-clock timestamps.
type Sample struct {
	ID            string                   `json:"id"`
	Provider      string                   `json:"provider"`
	Region        string                   `json:"region,omitempty"`
	Outcome       Outcome                  `json:"outcome"`
	CacheObserved bool                     `json:"cache_observed"`
	CacheHit      bool                     `json:"cache_hit"`
	Timings       map[string]time.Duration `json:"-"`
	Resources     Resources                `json:"resources"`
}

// Resources describes the capacity requested by the observed job.
type Resources struct {
	CPU      int     `json:"cpu"`
	MemoryGB float64 `json:"memory_gb"`
	GPU      int     `json:"gpu"`
}

// DurationStats contains JSON-safe millisecond values. P95 uses the
// nearest-rank definition (ceil(0.95*n)), with no interpolation.
type DurationStats struct {
	Count    int     `json:"count"`
	MeanMS   float64 `json:"mean_ms"`
	MedianMS float64 `json:"median_ms"`
	P95MS    float64 `json:"p95_ms"`
}

// ResourceSummary is an aggregate over the requested resource dimensions.
type ResourceSummary struct {
	Count        int     `json:"count"`
	MeanCPU      float64 `json:"mean_cpu"`
	MaxCPU       int     `json:"max_cpu"`
	MeanMemoryGB float64 `json:"mean_memory_gb"`
	MaxMemoryGB  float64 `json:"max_memory_gb"`
	MeanGPU      float64 `json:"mean_gpu"`
	MaxGPU       int     `json:"max_gpu"`
}

// ProviderPerformance contains outcome and timing metrics for one provider.
type ProviderPerformance struct {
	Count          int                      `json:"count"`
	Successes      int                      `json:"successes"`
	Failures       int                      `json:"failures"`
	FailureRate    float64                  `json:"failure_rate"`
	Timings        DurationStats            `json:"timings"`
	TimingsByPhase map[string]DurationStats `json:"timings_by_phase"`
}

// Report is the deterministic result of Analyze. Maps are safe to marshal
// with encoding/json; encoding/json sorts string map keys during encoding.
type Report struct {
	Count               int                            `json:"count"`
	Successes           int                            `json:"successes"`
	Failures            int                            `json:"failures"`
	FailureRate         float64                        `json:"failure_rate"`
	CacheObserved       int                            `json:"cache_observed"`
	CacheHits           int                            `json:"cache_hits"`
	CacheHitRate        float64                        `json:"cache_hit_rate"`
	Timings             map[string]DurationStats       `json:"timings"`
	Providers           map[string]ProviderPerformance `json:"providers"`
	Resources           ResourceSummary                `json:"resources"`
	ResourcesByProvider map[string]ResourceSummary     `json:"resources_by_provider"`
}

// Analyze validates all samples and computes the report. The input is never
// mutated, and the result does not depend on sample order.
func Analyze(samples []Sample) (Report, error) {
	if len(samples) == 0 {
		return Report{}, ErrNoSamples
	}
	seen := make(map[string]struct{}, len(samples))
	for i, sample := range samples {
		if err := validateSample(sample); err != nil {
			return Report{}, fmt.Errorf("sample %d: %w", i, err)
		}
		if _, ok := seen[sample.ID]; ok {
			return Report{}, fmt.Errorf("%w: %q", ErrDuplicateSample, sample.ID)
		}
		seen[sample.ID] = struct{}{}
	}
	orderedSamples := append([]Sample(nil), samples...)
	sort.Slice(orderedSamples, func(i, j int) bool { return orderedSamples[i].ID < orderedSamples[j].ID })

	report := Report{
		Count: len(samples), Timings: make(map[string]DurationStats),
		Providers:           make(map[string]ProviderPerformance),
		ResourcesByProvider: make(map[string]ResourceSummary),
	}
	phaseValues := make(map[string][]time.Duration)
	providerValues := make(map[string][]time.Duration)
	providerPhaseValues := make(map[string]map[string][]time.Duration)
	providerCounts := make(map[string][2]int) // successes, failures
	providerResources := make(map[string][]Resources)
	allResources := make([]Resources, 0, len(samples))

	for _, sample := range orderedSamples {
		if sample.Outcome == OutcomeSuccess {
			report.Successes++
		} else {
			report.Failures++
		}
		if sample.CacheObserved {
			report.CacheObserved++
			if sample.CacheHit {
				report.CacheHits++
			}
		}
		for phase, duration := range sample.Timings {
			phaseValues[phase] = append(phaseValues[phase], duration)
			providerValues[sample.Provider] = append(providerValues[sample.Provider], duration)
			if providerPhaseValues[sample.Provider] == nil {
				providerPhaseValues[sample.Provider] = make(map[string][]time.Duration)
			}
			providerPhaseValues[sample.Provider][phase] = append(providerPhaseValues[sample.Provider][phase], duration)
		}
		counts := providerCounts[sample.Provider]
		if sample.Outcome == OutcomeSuccess {
			counts[0]++
		} else {
			counts[1]++
		}
		providerCounts[sample.Provider] = counts
		providerResources[sample.Provider] = append(providerResources[sample.Provider], sample.Resources)
		allResources = append(allResources, sample.Resources)
	}

	report.FailureRate = rate(report.Failures, report.Count)
	report.CacheHitRate = rate(report.CacheHits, report.CacheObserved)
	for phase, values := range phaseValues {
		report.Timings[phase] = summarizeDurations(values)
	}
	for provider, counts := range providerCounts {
		report.Providers[provider] = ProviderPerformance{
			Count: counts[0] + counts[1], Successes: counts[0], Failures: counts[1],
			FailureRate:    rate(counts[1], counts[0]+counts[1]),
			Timings:        summarizeDurations(providerValues[provider]),
			TimingsByPhase: summarizePhaseDurations(providerPhaseValues[provider]),
		}
		report.ResourcesByProvider[provider] = summarizeResources(providerResources[provider])
	}
	report.Resources = summarizeResources(allResources)
	return report, nil
}

func validateSample(sample Sample) error {
	if strings.TrimSpace(sample.ID) == "" || strings.TrimSpace(sample.Provider) == "" {
		return fmt.Errorf("%w: id and provider are required", ErrInvalidSample)
	}
	switch sample.Outcome {
	case OutcomeSuccess, OutcomeFailure, OutcomeCancelled:
	default:
		return fmt.Errorf("%w: outcome must be success, failure, or cancelled", ErrInvalidSample)
	}
	if sample.CacheHit && !sample.CacheObserved {
		return fmt.Errorf("%w: cache_hit requires cache_observed", ErrInvalidSample)
	}
	if sample.Resources.CPU < 0 || sample.Resources.GPU < 0 || sample.Resources.MemoryGB < 0 || math.IsNaN(sample.Resources.MemoryGB) || math.IsInf(sample.Resources.MemoryGB, 0) {
		return fmt.Errorf("%w: resources must be finite and non-negative", ErrInvalidSample)
	}
	for name, duration := range sample.Timings {
		if strings.TrimSpace(name) == "" || duration < 0 {
			return fmt.Errorf("%w: timing names are required and durations cannot be negative", ErrInvalidSample)
		}
	}
	return nil
}

func summarizeDurations(values []time.Duration) DurationStats {
	if len(values) == 0 {
		return DurationStats{}
	}
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	mean := 0.0
	for i, value := range ordered {
		mean += (durationMS(value) - mean) / float64(i+1)
	}
	return DurationStats{
		Count: len(ordered), MeanMS: mean,
		MedianMS: durationMS(median(ordered)), P95MS: durationMS(ordered[(len(ordered)*95+99)/100-1]),
	}
}

func summarizePhaseDurations(values map[string][]time.Duration) map[string]DurationStats {
	result := make(map[string]DurationStats, len(values))
	for phase, durations := range values {
		result[phase] = summarizeDurations(durations)
	}
	return result
}

func median(values []time.Duration) time.Duration {
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return values[middle-1] + (values[middle]-values[middle-1])/2
}

func summarizeResources(values []Resources) ResourceSummary {
	if len(values) == 0 {
		return ResourceSummary{}
	}
	result := ResourceSummary{Count: len(values)}
	for i, value := range values {
		position := float64(i + 1)
		result.MeanCPU += (float64(value.CPU) - result.MeanCPU) / position
		result.MeanMemoryGB += (value.MemoryGB - result.MeanMemoryGB) / position
		result.MeanGPU += (float64(value.GPU) - result.MeanGPU) / position
		if value.CPU > result.MaxCPU {
			result.MaxCPU = value.CPU
		}
		if value.MemoryGB > result.MaxMemoryGB {
			result.MaxMemoryGB = value.MemoryGB
		}
		if value.GPU > result.MaxGPU {
			result.MaxGPU = value.GPU
		}
	}
	return result
}

func rate(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func durationMS(value time.Duration) float64 { return float64(value) / float64(time.Millisecond) }

// MarshalJSON is provided on Sample so structured samples are easy to persist
// and exchange without exposing Go's nanosecond encoding for time.Duration.
func (s Sample) MarshalJSON() ([]byte, error) {
	timings := make(map[string]float64, len(s.Timings))
	for name, value := range s.Timings {
		timings[name] = durationMS(value)
	}
	type wire struct {
		ID            string             `json:"id"`
		Provider      string             `json:"provider"`
		Region        string             `json:"region,omitempty"`
		Outcome       Outcome            `json:"outcome"`
		CacheObserved bool               `json:"cache_observed"`
		CacheHit      bool               `json:"cache_hit"`
		Timings       map[string]float64 `json:"timings_ms"`
		Resources     Resources          `json:"resources"`
	}
	return json.Marshal(wire{s.ID, s.Provider, s.Region, s.Outcome, s.CacheObserved, s.CacheHit, timings, s.Resources})
}
