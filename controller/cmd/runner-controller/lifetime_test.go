package main

import (
	"testing"
	"time"
)

func TestRunnerLifetimeFallsBackToTwoHours(t *testing.T) {
	if got := runnerLifetime(0); got != 2*time.Hour {
		t.Fatalf("zero = %s", got)
	}
	if got := runnerLifetime(45 * time.Minute); got != 45*time.Minute {
		t.Fatalf("configured = %s", got)
	}
}
