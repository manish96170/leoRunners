package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validInputs() Inputs {
	return Inputs{Provider: "aws", Region: "us-east-1", InstanceType: "t3.small", Currency: "USD", HourlyPrice: .02, DurationHours: 2.5, StorageGB: 10, StorageHourlyPrice: .0001, NetworkGB: 4, NetworkPricePerGB: .09, CacheGB: 2, CacheHourlyPrice: .0002}
}

func TestEstimateItemizesAndRounds(t *testing.T) {
	r, err := estimate(validInputs(), time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 4 || r.Total != .41 {
		t.Fatalf("items=%d total=%v", len(r.Items), r.Total)
	}
	if r.EvidenceStatus != "estimate-not-actual-billing-evidence" {
		t.Fatal(r.EvidenceStatus)
	}
}

func TestValidationRejectsMissingAndNegative(t *testing.T) {
	in := validInputs()
	in.Provider = ""
	if err := validate(in); err == nil {
		t.Fatal("missing provider accepted")
	}
	in = validInputs()
	in.NetworkGB = -1
	if err := validate(in); err == nil {
		t.Fatal("negative network accepted")
	}
	in = validInputs()
	in.DurationHours = -1
	if err := validate(in); err == nil {
		t.Fatal("missing duration accepted")
	}
}

func TestZeroCostInputsAreValid(t *testing.T) {
	in := validInputs()
	in.StorageGB, in.StorageHourlyPrice, in.NetworkGB, in.NetworkPricePerGB, in.CacheGB, in.CacheHourlyPrice = 0, 0, 0, 0, 0, 0
	if _, err := estimate(in, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestJSONOutputAndMarkdownLabel(t *testing.T) {
	r, _ := estimate(validInputs(), time.Now())
	var jsonOut bytes.Buffer
	if err := writeJSON(&jsonOut, r); err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(jsonOut.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	var md bytes.Buffer
	if err := writeMarkdown(&md, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md.String(), "Estimate only; not actual billing evidence") || !strings.Contains(md.String(), "Total") {
		t.Fatal(md.String())
	}
}
