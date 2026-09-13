package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testDurations() map[Phase]time.Duration {
	return map[Phase]time.Duration{Queue: 1 * time.Second, Launch: 2 * time.Second, Boot: 3 * time.Second, Registration: 4 * time.Second, Ready: 5 * time.Second, JobStart: 6 * time.Second, Cleanup: 7 * time.Second}
}

func TestFakeTimelineReplayIsDeterministic(t *testing.T) {
	timeline := FakeTimeline{ID: "job-1", Start: time.Unix(100, 0), Durations: testDurations()}
	first, err := timeline.Replay()
	if err != nil {
		t.Fatal(err)
	}
	second, err := timeline.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if string(mustJSON(first)) != string(mustJSON(second)) {
		t.Fatal("replay changed between runs")
	}
	if got, want := first.Duration(Ready), 5*time.Second; got != want {
		t.Fatalf("ready duration = %s, want %s", got, want)
	}
}

func TestCollectorRejectsOutOfOrderMark(t *testing.T) {
	c := NewCollector(time.Unix(100, 0))
	if err := c.Mark(Queue, time.Unix(99, 0)); err == nil {
		t.Fatal("expected timestamp validation error")
	}
}

func TestOutputContainsAllPhases(t *testing.T) {
	sample, err := (FakeTimeline{ID: "job-1", Durations: testDurations()}).Replay()
	if err != nil {
		t.Fatal(err)
	}
	var jsonOutput bytes.Buffer
	if err := WriteJSON(&jsonOutput, []Sample{sample}); err != nil {
		t.Fatal(err)
	}
	var decoded []Sample
	if err := json.Unmarshal(jsonOutput.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 1 || len(decoded[0].Durations) != len(phases) {
		t.Fatalf("JSON omitted phase durations: %s", jsonOutput.String())
	}
	var csvOutput bytes.Buffer
	if err := WriteCSV(&csvOutput, []Sample{sample}); err != nil {
		t.Fatal(err)
	}
	for _, phase := range phases {
		if !strings.Contains(csvOutput.String(), string(phase)+"_ms") {
			t.Fatalf("CSV omitted %s", phase)
		}
	}
}

func mustJSON(sample Sample) []byte { data, _ := json.Marshal(sample); return data }
