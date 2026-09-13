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
