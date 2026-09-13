package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("fixtures", name)
}

func load(t *testing.T) (Policy, Contract) {
	t.Helper()
	var p Policy
	var c Contract
	for path, dst := range map[string]interface{}{fixture(t, "policy-safe.json"): &p, fixture(t, "contract-safe.json"): &c} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, dst); err != nil {
			t.Fatal(err)
		}
	}
	return p, c
}

func TestSafeContract(t *testing.T) {
	p, c := load(t)
	if got := validate(p, c); len(got) != 0 {
		t.Fatalf("safe policy failed: %v", got)
	}
}

func TestUnsafePolicyFails(t *testing.T) {
	var p Policy
	data, err := os.ReadFile(fixture(t, "policy-unsafe.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	_, c := load(t)
	if got := validate(p, c); len(got) == 0 {
		t.Fatal("unsafe policy passed")
	}
}

func TestPassRoleRequiresCondition(t *testing.T) {
	p, c := load(t)
	ss, _ := statements(p.Statement)
	ss[1].Condition = nil
	p.Statement = ss
	if got := validate(p, c); len(got) == 0 {
		t.Fatal("PassRole without condition passed")
	}
}

func TestWildcardMatcher(t *testing.T) {
	if !matches("arn:aws:iam::123:role/leo-runner-*", "arn:aws:iam::123:role/leo-runner-prod") {
		t.Fatal("expected wildcard match")
	}
	if matches("arn:aws:iam::123:role/leo-runner-*", "arn:aws:iam::123:role/admin") {
		t.Fatal("unexpected wildcard match")
	}
}
