package github

import (
	"net/http"
	"testing"
	"time"
)

func TestNormalizeWorkflowJob(t *testing.T) {
	payload := []byte(`{"action":"completed","workflow_job":{"id":42,"run_id":7,"run_attempt":2,"name":"test","status":"completed","conclusion":"cancelled","labels":["self-hosted","linux"],"repository":{"id":9,"full_name":"acme/widgets","owner":{"login":"acme"},"name":"widgets"}}}`)
	got, err := NormalizeWorkflowJob(payload, "delivery-123", time.Date(2026, 9, 13, 1, 2, 3, 0, time.FixedZone("IST", 19800)))
	if err != nil {
		t.Fatal(err)
	}
	if got.EventName != WorkflowJobEventName || got.Action != ActionCompleted || got.EventID != "delivery-123" {
		t.Fatalf("unexpected event identity: %+v", got)
	}
	if got.Job.ID != 42 || got.Job.Conclusion != "cancelled" || got.Job.Labels[1] != "linux" {
		t.Fatalf("unexpected job: %+v", got.Job)
	}
	if got.JobKey != "acme/widgets/job/42" || got.RunKey != "acme/widgets/run/7/attempt/2" || got.DedupKey != "delivery-123" {
		t.Fatalf("unexpected dedup fields: %+v", got)
	}
	if got.ReceivedAt.Location() != time.UTC {
		t.Fatalf("received time is not UTC: %v", got.ReceivedAt.Location())
	}
}

func TestNormalizeWorkflowJobSupportsLifecycleActions(t *testing.T) {
	for _, action := range []string{"queued", "in_progress", "completed", "cancelled"} {
		payload := []byte(`{"action":"` + action + `","workflow_job":{"id":1,"run_id":2}}`)
		if _, err := NormalizeWorkflowJob(payload, "", time.Time{}); err != nil {
			t.Errorf("action %q: %v", action, err)
		}
	}
}

func TestNormalizeWorkflowJobRejectsInvalidPayload(t *testing.T) {
	for _, payload := range []string{
		`{"action":"requested","workflow_job":{"id":1,"run_id":2}}`,
		`{"action":"queued","workflow_job":{"id":0,"run_id":2}}`,
		`{"action":"queued","workflow_job":{"id":1,"run_id":0}}`,
		`not-json`,
	} {
		if _, err := NormalizeWorkflowJob([]byte(payload), "", time.Time{}); err == nil {
			t.Errorf("payload %q was accepted", payload)
		}
	}
}

func TestNormalizeWorkflowJobUsesFallbackDedupKey(t *testing.T) {
	payload := []byte(`{"action":"queued","workflow_job":{"id":4,"run_id":5,"run_attempt":1,"repository":{"full_name":"acme/widgets"}}}`)
	got, err := NormalizeWorkflowJob(payload, "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got.EventID != "acme/widgets/job/4/action/queued" || got.DedupKey != got.EventID {
		t.Fatalf("unexpected fallback event ID: %+v", got)
	}
}

func TestNormalizeWorkflowJobHeadersUsesTopLevelRepository(t *testing.T) {
	payload := []byte(`{"action":"queued","workflow_job":{"id":4,"run_id":5,"run_attempt":1,"labels":["self-hosted"]},"repository":{"id":9,"full_name":"acme/widgets","owner":"acme","name":"widgets"}}`)
	headers := http.Header{
		"X-GitHub-Delivery": []string{"delivery-456"},
		"X-GitHub-Event":    []string{"workflow_job"},
	}
	got, err := NormalizeWorkflowJobHeaders(payload, headers, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got.EventID != "delivery-456" || got.Job.Repository.FullName != "acme/widgets" || got.Job.Labels[0] != "self-hosted" {
		t.Fatalf("unexpected normalized event: %+v", got)
	}
}

func TestNormalizeWorkflowJobHeadersRejectsOtherEvents(t *testing.T) {
	_, err := NormalizeWorkflowJobHeaders([]byte(`{}`), http.Header{"X-GitHub-Event": []string{"push"}}, time.Time{})
	if err == nil {
		t.Fatal("non-workflow_job event was accepted")
	}
}
