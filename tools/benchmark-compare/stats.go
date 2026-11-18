package main

import (
	"sort"
	"time"
)

func calculateStats(observations []Observation) map[string]Stats {
	values := make(map[string][]time.Duration)
	for _, observation := range observations {
		values[observation.Metric] = append(values[observation.Metric], observation.Duration)
	}
	result := make(map[string]Stats, len(values))
	for metric, durations := range values {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		result[metric] = Stats{Count: len(durations), MinMS: ms(durations[0]), MedianMS: ms(medianDuration(durations)), P95MS: ms(percentile95(durations)), MaxMS: ms(durations[len(durations)-1])}
	}
	return result
}

func medianDuration(values []time.Duration) time.Duration {
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return values[middle-1] + (values[middle]-values[middle-1])/2
}

func percentile95(values []time.Duration) time.Duration {
	index := (len(values)*95+99)/100 - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func ms(value time.Duration) float64 { return float64(value) / float64(time.Millisecond) }
