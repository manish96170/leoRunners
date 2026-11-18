package domain

import (
	"testing"
	"time"
)

func TestTransitionRejectsRegression(t *testing.T) {
	j := Job{ID: "j", State: JobQueued}
	if err := Transition(&j, JobProvisioning, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := Transition(&j, JobQueued, time.Now()); err != ErrInvalidTransition {
		t.Fatalf("got %v", err)
	}
}
