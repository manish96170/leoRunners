package main

import "testing"

func TestVersionInRange(t *testing.T) {
	if !versionInRange("6.64.0", "5.0.0", "7.0.0") {
		t.Fatal("expected provider version to be in range")
	}
	if versionInRange("7.0.0", "5.0.0", "7.0.0") {
		t.Fatal("expected exclusive upper bound")
	}
}

func TestVersionInConstraint(t *testing.T) {
	if !versionInConstraint("6.64.0", ">= 5.0, < 7.0") {
		t.Fatal("expected version to satisfy constraint")
	}
	if versionInConstraint("4.99.0", ">= 5.0") {
		t.Fatal("expected lower bound rejection")
	}
	if versionInConstraint("7.0.0", ">= 5.0, < 7.0") {
		t.Fatal("expected upper bound rejection")
	}
}

func TestContractRootWiring(t *testing.T) {
	text := `output "state_table_name" {
  value = module.controller_state.table_name
}`
	if got := namedBlock(text, "output", "state_table_name"); got == "" {
		t.Fatal("expected output block to be extracted")
	}
}

func TestIAMRoleReferenceAllowsCountIndex(t *testing.T) {
	text := `resource "aws_iam_role" "task" {
  count = 1
}

resource "aws_iam_role_policy" "runtime" {
  count = 1
  role  = aws_iam_role.task[0].name
}

resource "aws_iam_role_policy_attachment" "runtime" {
  for_each = toset(["a"])
  role     = aws_iam_role.task[0].name
}`
	var failures []string
	checkResourceDependencies(text, "modules/example", func(status, _, _, _ string) {
		if status == "FAIL" {
			failures = append(failures, status)
		}
	})
	if len(failures) != 0 {
		t.Fatalf("count-indexed role references must be accepted, got %d failures", len(failures))
	}
}

func TestIAMRoleReferenceStillRequiresDeclaredRole(t *testing.T) {
	text := `resource "aws_iam_role_policy" "runtime" {
  role = aws_iam_role.missing[0].name
}`
	var failures int
	checkResourceDependencies(text, "modules/example", func(status, _, _, _ string) {
		if status == "FAIL" {
			failures++
		}
	})
	if failures != 1 {
		t.Fatalf("an undeclared role must still fail, got %d failures", failures)
	}
}
