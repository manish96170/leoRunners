# Transparent Cost Estimator

This standalone Go tool produces an itemized cost estimate from caller-supplied prices. It does not fetch live pricing, call AWS, or claim to reproduce provider billing.

## Usage

Every quantity and rate is explicit. Zero is valid; omitted and negative values are rejected.

```sh
go run . \
  --provider aws --region us-east-1 --instance-type t3.small \
  --currency USD --hourly-price 0.0208 --duration 2.5 \
  --storage-gb 40 --storage-hourly-price 0.00011 \
  --network-gb 3 --network-price-per-gb 0.09 \
  --cache-gb 2 --cache-hourly-price 0.0002 \
  --format markdown
```

Use `--format json` for machine-readable output, `--output report.json` to write a file, and repeat `--assumption "..."` to document workload-specific assumptions. Amounts are rounded to cents per line item and totalled from those rounded line items.

The report includes `report_type: cost-estimate`, `evidence_status: estimate-not-actual-billing-evidence`, all source inputs, formulas, itemized amounts, currency, and assumptions. Actual billing reconciliation should be performed separately from provider billing exports.
