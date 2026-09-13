#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
report=${FINAL_READINESS_REPORT:-}
tmp=$(mktemp -d "${TMPDIR:-/tmp}/leo-final-readiness.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
results="$tmp/results.tsv"
: >"$results"
overall=0

record() {
  local name=$1 status=$2 message=$3
  printf '%s\t%s\t%s\n' "$name" "$status" "$message" >>"$results"
  printf '%-24s %s\n' "$status $name" "$message"
  [ "$status" = PASS ] || overall=1
}

run() {
  local name=$1; shift
  local log="$tmp/$name.log"
  if "$@" >"$log" 2>&1; then
    record "$name" PASS "offline validation passed"
  else
    record "$name" BLOCKED "offline validation failed or prerequisite unavailable"
  fi
}

run controller bash -c "export PATH=/tmp/leo-go1.27.1/bin:\$PATH; cd '$root/controller'; go test ./...; go vet ./...; go build ./..."
run extensions "$root/tools/extension-validation/test.sh"
run intelligence "$root/intelligence/validate.sh"
run github "$root/tools/github-validation/test.sh"
run ami "$root/ami/test.sh"
run network "$root/tools/network-validation/test.sh"
run events "$root/tools/event-sink-validation/test.sh"
run workloads "$root/workloads/validate.sh"
run benchmarks "$root/benchmarks/validate.sh"
run docker "$root/tools/docker-validation/test.sh"
run kubernetes "$root/deploy/kubernetes/test.sh"
run iam "$root/tools/iam-validation/test.sh"
run terraform "$root/tools/terraform-validation/test.sh"
run release "$root/tools/release-validation/test.sh"

if grep -R -Eiq 'run-instances|instances create|generate-jitconfig|terraform apply|kubectl apply' "$tmp"; then
  record live-mutation BLOCKED "mutation command appeared in readiness output"
else
  record live-mutation PASS "no live mutation command was invoked"
fi

status=pass
[ "$overall" -eq 0 ] || status=blocked
if [ -n "$report" ]; then
  mkdir -p "$(dirname -- "$report")"
  ruby -rjson - "$results" "$report" "$status" <<'RUBY'
results, destination, status = ARGV
checks = File.readlines(results, chomp: true).map do |line|
  name, check_status, message = line.split("\t", 3)
  {"name" => name, "status" => check_status, "message" => message.to_s}
end
document = {"apiVersion" => "readiness.leorunners.io/v1", "kind" => "FinalReadinessReport", "metadata" => {"name" => "leo-runners", "version" => "1.0.0"}, "spec" => {"status" => status, "read_only" => true, "live_mutation_invoked" => false, "checks" => checks, "environment_gated" => %w[AWS GCP GitHub Docker kubectl Packer Terraform-provider]}}
tmp = "#{destination}.tmp.#{$$}"
File.open(tmp, "w", 0o600) { |f| f.write(JSON.pretty_generate(document) + "\n") }
File.rename(tmp, destination)
RUBY
fi
exit "$overall"
