#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
failures=0

check() {
  local label=$1
  shift
  if "$@"; then
    printf 'PASS %s\n' "$label"
  else
    printf 'FAIL %s\n' "$label" >&2
    failures=$((failures + 1))
  fi
}

check "terraform files present" test -f "$root/main.tf"
check "log subscription is opt-in" rg -q 'var\.log_subscription == null' "$root/main.tf"
check "subscription has explicit destination" rg -q 'destination_arn = each\.value\.destination_arn' "$root/main.tf"
check "subscription has explicit filter" rg -q 'filter_pattern  = each\.value\.filter_pattern' "$root/main.tf"
check "subscription validates ARN and filter" rg -q 'var\.log_subscription\.filter_pattern\)\) > 0' "$root/variables.tf"
check "notification-only alarms" rg -q 'alarm_actions = var\.alarm_actions' "$root/main.tf"
check "bounded dimensions" rg -q 'length\(var\.metric_dimensions\) <= 3' "$root/variables.tf"
check "approved dimension names" rg -q 'Environment.*Provider.*Region' "$root/variables.tf"
check "supported retention validation" rg -q 'CloudWatch-supported' "$root/variables.tf"
check "encryption gate" rg -q 'require_encryption=true requires' "$root/variables.tf"
check "explicit missing data" rg -q 'treat_missing_data' "$root/main.tf"
check "p99 duration alarm" bash -c "rg -q 'LifecycleDuration' '$root/variables.tf' && rg -q 'p99' '$root/variables.tf'"

if rg -n 'aws_iam_|aws_lambda_function|aws_autoscaling_|aws_instance|aws_ec2_' "$root" --glob '*.tf' >/dev/null; then
  printf 'FAIL forbidden IAM or lifecycle resources found\n' >&2
  failures=$((failures + 1))
else
  printf 'PASS no IAM or lifecycle resources\n'
fi

if [[ $failures -gt 0 ]]; then
  printf '%d observability module check(s) failed\n' "$failures" >&2
  exit 1
fi
printf 'Observability module validation passed\n'
