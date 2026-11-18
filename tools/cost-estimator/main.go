package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"
)

type assumptionsFlag []string

func (a *assumptionsFlag) String() string { return strings.Join(*a, " | ") }
func (a *assumptionsFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("assumption cannot be empty")
	}
	*a = append(*a, value)
	return nil
}

type Inputs struct {
	Provider           string   `json:"provider"`
	Region             string   `json:"region"`
	InstanceType       string   `json:"instance_type"`
	Currency           string   `json:"currency"`
	HourlyPrice        float64  `json:"hourly_price"`
	DurationHours      float64  `json:"duration_hours"`
	StorageGB          float64  `json:"storage_gb"`
	StorageHourlyPrice float64  `json:"storage_hourly_price"`
	NetworkGB          float64  `json:"network_gb"`
	NetworkPricePerGB  float64  `json:"network_price_per_gb"`
	CacheGB            float64  `json:"cache_gb"`
	CacheHourlyPrice   float64  `json:"cache_hourly_price"`
	Assumptions        []string `json:"assumptions"`
}

type LineItem struct {
	Name      string  `json:"name"`
	Quantity  float64 `json:"quantity"`
	Unit      string  `json:"unit"`
	UnitPrice float64 `json:"unit_price"`
	PriceUnit string  `json:"price_unit"`
	Amount    float64 `json:"amount"`
	Formula   string  `json:"formula"`
}

type Report struct {
	ReportType     string     `json:"report_type"`
	EvidenceStatus string     `json:"evidence_status"`
	GeneratedAt    string     `json:"generated_at"`
	Provider       string     `json:"provider"`
	Region         string     `json:"region"`
	InstanceType   string     `json:"instance_type"`
	Currency       string     `json:"currency"`
	Inputs         Inputs     `json:"inputs"`
	Items          []LineItem `json:"items"`
	Total          float64    `json:"total"`
	Assumptions    []string   `json:"assumptions"`
	Disclaimer     string     `json:"disclaimer"`
}

func validate(in Inputs) error {
	for name, value := range map[string]float64{
		"hourly-price": in.HourlyPrice, "duration": in.DurationHours,
		"storage-gb": in.StorageGB, "storage-hourly-price": in.StorageHourlyPrice,
		"network-gb": in.NetworkGB, "network-price-per-gb": in.NetworkPricePerGB,
		"cache-gb": in.CacheGB, "cache-hourly-price": in.CacheHourlyPrice,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return fmt.Errorf("%s must be a finite non-negative number", name)
		}
	}
	for name, value := range map[string]string{"provider": in.Provider, "region": in.Region, "instance-type": in.InstanceType, "currency": in.Currency} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	return nil
}

func cents(value float64) float64 { return math.Round(value*100) / 100 }

func estimate(in Inputs, now time.Time) (Report, error) {
	if err := validate(in); err != nil {
		return Report{}, err
	}
	items := []LineItem{
		{Name: "compute", Quantity: in.DurationHours, Unit: "hour", UnitPrice: in.HourlyPrice, PriceUnit: "currency/hour", Amount: cents(in.DurationHours * in.HourlyPrice), Formula: "duration_hours * hourly_price"},
		{Name: "storage", Quantity: in.StorageGB * in.DurationHours, Unit: "GB-hour", UnitPrice: in.StorageHourlyPrice, PriceUnit: "currency/GB-hour", Amount: cents(in.StorageGB * in.DurationHours * in.StorageHourlyPrice), Formula: "storage_gb * duration_hours * storage_hourly_price"},
		{Name: "network", Quantity: in.NetworkGB, Unit: "GB", UnitPrice: in.NetworkPricePerGB, PriceUnit: "currency/GB", Amount: cents(in.NetworkGB * in.NetworkPricePerGB), Formula: "network_gb * network_price_per_gb"},
		{Name: "cache", Quantity: in.CacheGB * in.DurationHours, Unit: "GB-hour", UnitPrice: in.CacheHourlyPrice, PriceUnit: "currency/GB-hour", Amount: cents(in.CacheGB * in.DurationHours * in.CacheHourlyPrice), Formula: "cache_gb * duration_hours * cache_hourly_price"},
	}
	total := 0.0
	for _, item := range items {
		total = cents(total + item.Amount)
	}
	assumptions := append([]string{}, in.Assumptions...)
	if len(assumptions) == 0 {
		assumptions = []string{"Prices and quantities were supplied explicitly by the caller.", "Storage and cache are modeled for the full duration; network is modeled as egress or transfer GB only."}
	}
	in.Assumptions = assumptions
	return Report{ReportType: "cost-estimate", EvidenceStatus: "estimate-not-actual-billing-evidence", GeneratedAt: now.UTC().Format(time.RFC3339), Provider: in.Provider, Region: in.Region, InstanceType: in.InstanceType, Currency: in.Currency, Inputs: in, Items: items, Total: total, Assumptions: assumptions, Disclaimer: "This is a transparent estimate calculated from explicit inputs. It does not fetch live pricing and is not actual provider billing evidence."}, nil
}

func writeJSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func writeMarkdown(w io.Writer, r Report) error {
	f := func(v float64) string { return fmt.Sprintf("%.2f %s", v, r.Currency) }
	rate := func(v float64) string { return fmt.Sprintf("%g %s", v, r.Currency) }
	fmt.Fprintf(w, "# Cost Estimate\n\n")
	fmt.Fprintf(w, "**Status:** Estimate only; not actual billing evidence.\n\n")
	fmt.Fprintf(w, "| Field | Value |\n|---|---|\n| Provider | %s |\n| Region | %s |\n| Instance type | %s |\n| Currency | %s |\n| Generated | %s |\n\n", r.Provider, r.Region, r.InstanceType, r.Currency, r.GeneratedAt)
	fmt.Fprintln(w, "| Item | Quantity | Unit price | Amount | Formula |\n|---|---:|---:|---:|---|")
	for _, item := range r.Items {
		fmt.Fprintf(w, "| %s | %.4g %s | %s/%s | %s | `%s` |\n", item.Name, item.Quantity, item.Unit, rate(item.UnitPrice), item.PriceUnit, f(item.Amount), item.Formula)
	}
	fmt.Fprintf(w, "\n**Total: %s**\n\n## Assumptions\n\n", f(r.Total))
	for _, a := range r.Assumptions {
		fmt.Fprintf(w, "- %s\n", a)
	}
	fmt.Fprintf(w, "\n> %s\n", r.Disclaimer)
	return nil
}

func main() {
	var in Inputs
	var format, output string
	var assumptions assumptionsFlag
	flag.StringVar(&in.Provider, "provider", "", "provider name (required)")
	flag.StringVar(&in.Region, "region", "", "provider region (required)")
	flag.StringVar(&in.InstanceType, "instance-type", "", "instance type (required)")
	flag.StringVar(&in.Currency, "currency", "", "currency code (required)")
	flag.Float64Var(&in.HourlyPrice, "hourly-price", -1, "compute price per hour (required)")
	flag.Float64Var(&in.DurationHours, "duration", -1, "duration in hours (required)")
	flag.Float64Var(&in.StorageGB, "storage-gb", -1, "storage quantity in GB (required; zero allowed)")
	flag.Float64Var(&in.StorageHourlyPrice, "storage-hourly-price", -1, "storage price per GB-hour (required; zero allowed)")
	flag.Float64Var(&in.NetworkGB, "network-gb", -1, "network quantity in GB (required; zero allowed)")
	flag.Float64Var(&in.NetworkPricePerGB, "network-price-per-gb", -1, "network price per GB (required; zero allowed)")
	flag.Float64Var(&in.CacheGB, "cache-gb", -1, "cache quantity in GB (required; zero allowed)")
	flag.Float64Var(&in.CacheHourlyPrice, "cache-hourly-price", -1, "cache price per GB-hour (required; zero allowed)")
	flag.Var(&assumptions, "assumption", "assumption (repeatable)")
	flag.StringVar(&format, "format", "json", "output format: json or markdown")
	flag.StringVar(&output, "output", "", "output file; stdout when omitted")
	flag.Parse()
	in.Assumptions = assumptions
	r, err := estimate(in, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	var w io.Writer = os.Stdout
	var file *os.File
	if output != "" {
		file, err = os.Create(output)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(2)
		}
		defer file.Close()
		w = file
	}
	switch strings.ToLower(format) {
	case "json":
		err = writeJSON(w, r)
	case "markdown", "md":
		err = writeMarkdown(w, r)
	default:
		err = fmt.Errorf("format must be json or markdown")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
}
