package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Policy struct {
	Version   string      `json:"Version"`
	Statement interface{} `json:"Statement"`
}

type Statement struct {
	Effect    string                 `json:"Effect"`
	Action    interface{}            `json:"Action"`
	Resource  interface{}            `json:"Resource"`
	Condition map[string]interface{} `json:"Condition"`
}

type Contract struct {
	Version           string              `json:"version"`
	ReviewOnly        bool                `json:"review_only"`
	RequiredActions   []string            `json:"required_actions"`
	ForbiddenActions  []string            `json:"forbidden_actions"`
	ActionResources   map[string][]string `json:"action_resources"`
	PassRoleServices  []string            `json:"pass_role_services"`
	DynamoDBTableARNs []string            `json:"dynamodb_table_arns"`
	DynamoDBIndexARNs []string            `json:"dynamodb_index_arns"`
	Simulations       []Simulation        `json:"simulations"`
}

type Simulation struct {
	Name     string            `json:"name"`
	Action   string            `json:"action"`
	Resource string            `json:"resource"`
	Context  map[string]string `json:"context"`
	Expect   string            `json:"expect"`
}

func list(v interface{}) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func readJSON(path string, dst interface{}) error {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func statements(raw interface{}) ([]Statement, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var one Statement
	if len(b) > 0 && b[0] == '{' {
		if err := json.Unmarshal(b, &one); err != nil {
			return nil, err
		}
		return []Statement{one}, nil
	}
	var many []Statement
	if err := json.Unmarshal(b, &many); err != nil {
		return nil, err
	}
	return many, nil
}

func wildcard(s string) bool {
	return s == "*" || strings.ContainsAny(s, "?[]") || strings.Contains(s, "*")
}

func matches(pattern, value string) bool {
	if pattern == value {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return false
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(value, parts[0]) || !strings.HasSuffix(value, parts[len(parts)-1]) {
		return false
	}
	pos := len(parts[0])
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(value[pos:], part)
		if i < 0 {
			return false
		}
		pos += i + len(part)
	}
	return true
}

func containsPattern(patterns []string, value string) bool {
	for _, p := range patterns {
		if matches(p, value) {
			return true
		}
	}
	return false
}

func contextValue(conditions map[string]interface{}, key string) []string {
	for operator, raw := range conditions {
		if operator != "StringEquals" && operator != "ForAllValues:StringEquals" {
			continue
		}
		if values, ok := raw.(map[string]interface{}); ok {
			if v, found := values[key]; found {
				return list(v)
			}
		}
	}
	return nil
}

func conditionAllows(s Statement, ctx map[string]string) bool {
	for operator, raw := range s.Condition {
		if operator != "StringEquals" && operator != "ForAllValues:StringEquals" {
			return false
		}
		values, ok := raw.(map[string]interface{})
		if !ok {
			return false
		}
		for key, expected := range values {
			actual, present := ctx[key]
			if !present {
				return false
			}
			if !containsPattern(list(expected), actual) {
				return false
			}
		}
	}
	return true
}

func allowed(policy []Statement, action, resource string, ctx map[string]string) bool {
	for _, s := range policy {
		if s.Effect != "Allow" || !containsPattern(list(s.Action), action) || !containsPattern(list(s.Resource), resource) || !conditionAllows(s, ctx) {
			continue
		}
		return true
	}
	return false
}

func validate(policy Policy, contract Contract) []string {
	var problems []string
	if policy.Version != "2012-10-17" {
		problems = append(problems, "policy Version must be 2012-10-17")
	}
	if contract.Version != "leo.iam-contract.v1" {
		problems = append(problems, "unsupported contract version")
	}
	if !contract.ReviewOnly {
		problems = append(problems, "contract must declare review_only=true")
	}
	ss, err := statements(policy.Statement)
	if err != nil {
		return append(problems, "invalid Statement: "+err.Error())
	}
	actions := map[string]bool{}
	for _, s := range ss {
		if s.Effect != "Allow" && s.Effect != "Deny" {
			problems = append(problems, "statement Effect must be Allow or Deny")
			continue
		}
		for _, action := range list(s.Action) {
			if wildcard(action) {
				problems = append(problems, "wildcard action is forbidden: "+action)
			}
			actions[action] = true
		}
		for _, resource := range list(s.Resource) {
			if resource == "*" {
				problems = append(problems, "wildcard resource is forbidden")
			}
			if strings.HasPrefix(resource, "arn:") && !strings.Contains(resource, ":") {
				problems = append(problems, "invalid resource ARN: "+resource)
			}
		}
		if containsPattern(list(s.Action), "iam:PassRole") {
			if len(contextValue(s.Condition, "iam:PassedToService")) == 0 {
				problems = append(problems, "iam:PassRole requires iam:PassedToService StringEquals condition")
			}
			for _, service := range contract.PassRoleServices {
				if !containsPattern(contextValue(s.Condition, "iam:PassedToService"), service) {
					problems = append(problems, "iam:PassRole does not allow required service: "+service)
				}
			}
		}
	}
	for _, action := range contract.RequiredActions {
		if !actions[action] {
			problems = append(problems, "required action missing: "+action)
		}
	}
	for _, action := range contract.ForbiddenActions {
		if actions[action] {
			problems = append(problems, "forbidden action present: "+action)
		}
	}
	for action, resources := range contract.ActionResources {
		for _, s := range ss {
			if s.Effect != "Allow" || !containsPattern(list(s.Action), action) {
				continue
			}
			for _, resource := range list(s.Resource) {
				if !containsPattern(resources, resource) {
					problems = append(problems, fmt.Sprintf("%s resource outside contract: %s", action, resource))
				}
			}
		}
	}
	for _, s := range ss {
		for _, action := range list(s.Action) {
			if !strings.HasPrefix(action, "dynamodb:") {
				continue
			}
			for _, resource := range list(s.Resource) {
				if strings.Contains(resource, "/index/") && !containsPattern(contract.DynamoDBIndexARNs, resource) {
					problems = append(problems, "DynamoDB index resource outside contract: "+resource)
				}
				if !strings.Contains(resource, "/index/") && !containsPattern(contract.DynamoDBTableARNs, resource) {
					problems = append(problems, "DynamoDB table resource outside contract: "+resource)
				}
			}
		}
	}
	for _, sim := range contract.Simulations {
		want := sim.Expect == "allow"
		got := allowed(ss, sim.Action, sim.Resource, sim.Context)
		if got != want {
			problems = append(problems, fmt.Sprintf("simulation %q expected %s, got %s", sim.Name, sim.Expect, map[bool]string{true: "allow", false: "deny"}[got]))
		}
	}
	return problems
}

func main() {
	policyPath := flag.String("policy", "", "policy JSON path")
	contractPath := flag.String("contract", "", "contract JSON path")
	flag.Parse()
	if *policyPath == "" || *contractPath == "" {
		fmt.Fprintln(os.Stderr, "usage: validate --policy PATH --contract PATH")
		os.Exit(2)
	}
	var policy Policy
	var contract Contract
	if err := readJSON(*policyPath, &policy); err != nil {
		fmt.Fprintln(os.Stderr, "policy:", err)
		os.Exit(2)
	}
	if err := readJSON(*contractPath, &contract); err != nil {
		fmt.Fprintln(os.Stderr, "contract:", err)
		os.Exit(2)
	}
	problems := validate(policy, contract)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "FAIL:", p)
		}
		os.Exit(1)
	}
	fmt.Println("IAM policy contract validation passed (review-only; no simulation API, upload, or apply performed)")
}
