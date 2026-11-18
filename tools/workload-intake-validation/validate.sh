#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
schema="$root/workload-intake/intake.schema.v1.json"
file=${1:-"$root/workload-intake/intake.v1.json"}
other=${2:-}

command -v jq >/dev/null 2>&1 || { echo "validate.sh: jq is required" >&2; exit 2; }
command -v shasum >/dev/null 2>&1 || { echo "validate.sh: shasum is required" >&2; exit 2; }
[ -f "$schema" ] || { echo "schema not found: $schema" >&2; exit 2; }
[ -f "$file" ] || { echo "intake not found: $file" >&2; exit 2; }
jq empty "$schema" >/dev/null || { echo "invalid schema JSON" >&2; exit 2; }
jq empty "$file" >/dev/null || { echo "invalid intake JSON: $file" >&2; exit 2; }

check_record() {
  record=$1
  jq -e '
    .apiVersion == "workload-intake.leorunners.io/v1" and
    .kind == "WorkloadIntake" and
    (.metadata.name | type == "string" and test("^[a-z][a-z0-9-]{2,63}$")) and
    (.metadata.version | type == "string" and test("^[0-9]+\\.[0-9]+\\.[0-9]+$")) and
    (.spec.repository.url | test("^https://github\\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")) and
    (.spec.repository.commit | test("^[0-9a-f]{40}$")) and
    (.spec.repository.workflow | test("^(?!/)(?!.*(?:^|/)\\.\\.(?:/|$))[A-Za-z0-9_.@/-]+\\.(yml|yaml)$")) and
    (.spec.repository.workflowDigest | test("^sha256:[0-9a-f]{64}$")) and
    (.spec.workload.id | test("^[a-z0-9][a-z0-9-]{2,63}$")) and
    (.spec.workload.manifestPath | test("^(?!/)(?!.*(?:^|/)\\.\\.(?:/|$))[^\\s]+$")) and
    (.spec.workload.manifestDigest | test("^sha256:[0-9a-f]{64}$")) and
    (.spec.workload.command | type == "string" and length >= 1 and length <= 240 and (test("[\\r\\n]") | not)) and
    (.spec.workload.commandDigest | test("^sha256:[0-9a-f]{64}$")) and
    (.spec.workload.shell | IN("bash", "sh", "pwsh")) and
    (.spec.provenance.source | IN("repository-defined", "controlled-intake", "offline-fixture")) and
    (.spec.provenance.inspectedAt | test("^20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")) and
    (.spec.provenance.submittedBy | test("^[a-z0-9][a-z0-9._-]{2,63}$")) and
    (.spec.provenance.approval.requestId | test("^req-[a-z0-9][a-z0-9-]{7,63}$")) and
    (.spec.provenance.approval.approvedBy | test("^[a-z0-9][a-z0-9._-]{2,63}$")) and
    (.spec.provenance.approval.approvedAt | test("^20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")) and
    (.spec.provenance.approval.expiresAt | test("^20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")) and
    (.spec.provenance.approval.purpose | type == "string" and length >= 3 and length <= 120) and
    (.spec.provenance.approval.expiresAt > .spec.provenance.approval.approvedAt) and
    (.spec.toolRequirements | type == "array" and length >= 1) and
    (all(.spec.toolRequirements[]; (.name | test("^[a-z0-9][a-z0-9._+-]{1,63}$")) and (.version | test("^[0-9]+\\.[0-9]+\\.[0-9]+$")) and (.source | IN("repository", "approved-image", "system")))) and
    ([.spec.toolRequirements[].name] | length == (unique | length)) and
    (.spec.observations.cache.mode | IN("disabled", "observe-only", "enabled")) and
    (.spec.observations.cache.namespaceObserved | type == "boolean") and
    (.spec.observations.cache.keysRedacted == true) and
    (.spec.observations.network.egressMode | IN("none", "nat", "vpc-endpoint", "hybrid", "approved-proxy")) and
    (.spec.observations.network.endpointsObserved | type == "array" and all(.[]; type == "string" and length <= 128)) and
    (.spec.observations.network.externalAccessApproved | type == "boolean") and
    ((.spec.observations.network.egressMode == "none") or (.spec.observations.network.externalAccessApproved == true)) and
    (.spec.observations.security.trustClass | IN("trusted-branch", "internal-pr", "fork", "untrusted")) and
    (.spec.observations.security.isolationProfile | IN("standard", "isolated", "privileged")) and
    (.spec.observations.security.capabilitiesObserved | type == "array" and all(.[]; test("^[a-z][a-z0-9-]{1,31}$"))) and
    ((.spec.observations.security.trustClass != "untrusted") or (.spec.observations.security.isolationProfile != "privileged")) and
    (.spec.redaction.status == "redacted") and (.spec.redaction.secretsScanned == true) and
    (.spec.redaction.rawPayloadsExcluded == true) and (.spec.redaction.identifiersHashed == true) and
    (.spec.comparability.protocolId | test("^workload-intake-[0-9]+$")) and
    (.spec.comparability.comparisonKey | test("^[a-z0-9][a-z0-9._:-]{2,127}$")) and
    (.spec.comparability.fixedCommit == .spec.repository.commit) and
    (.spec.comparability.fixedManifestDigest == .spec.workload.manifestDigest) and
    (.spec.comparability.fixedCommandDigest == .spec.workload.commandDigest) and
    ((tostring | test("(?i)(AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|bearer[[:space:]]+[A-Za-z0-9._-]+|password[=:][^[:space:]]+|secret[=:][^[:space:]]+|token[=:][^[:space:]]+|private[ _-]?key|BEGIN[[:space:]]+(RSA |EC |OPENSSH )?PRIVATE KEY)") | not))
  ' "$record" >/dev/null

  command=$(jq -r '.spec.workload.command' "$record")
  digest=$(printf '%s' "$command" | shasum -a 256 | awk '{print "sha256:" $1}')
  expected=$(jq -r '.spec.workload.commandDigest' "$record")
  [ "$digest" = "$expected" ] || { echo "command digest does not match command: $record" >&2; return 1; }

  if printf '%s' "$command" | grep -Eiq '(^|[[:space:];])while([[:space:]]|$)|(^|[[:space:];])until([[:space:]]|$)|(^|[[:space:]])(watch|tail[[:space:]]+-f|yes)([[:space:]]|$)|(^|[[:space:]])for[[:space:]].*in([[:space:]]|$)'; then
    echo "unbounded command rejected: $record" >&2
    return 1
  fi
}

if ! check_record "$file"; then
  echo "workload intake validation failed: $file" >&2
  exit 1
fi

if [ -n "$other" ]; then
  [ -f "$other" ] || { echo "comparison intake not found: $other" >&2; exit 2; }
  jq empty "$other" >/dev/null || { echo "invalid comparison JSON: $other" >&2; exit 2; }
  check_record "$other" || { echo "comparison workload intake validation failed: $other" >&2; exit 1; }
  jq -e -s '
    .[0].spec.repository.url == .[1].spec.repository.url and
    .[0].spec.repository.commit == .[1].spec.repository.commit and
    .[0].spec.repository.workflow == .[1].spec.repository.workflow and
    .[0].spec.repository.workflowDigest == .[1].spec.repository.workflowDigest and
    .[0].spec.workload.id == .[1].spec.workload.id and
    .[0].spec.workload.manifestDigest == .[1].spec.workload.manifestDigest and
    .[0].spec.workload.commandDigest == .[1].spec.workload.commandDigest and
    .[0].spec.comparability.protocolId == .[1].spec.comparability.protocolId and
    .[0].spec.comparability.comparisonKey == .[1].spec.comparability.comparisonKey and
    .[0].spec.comparability.fixedCommit == .[1].spec.comparability.fixedCommit and
    .[0].spec.comparability.fixedManifestDigest == .[1].spec.comparability.fixedManifestDigest and
    .[0].spec.comparability.fixedCommandDigest == .[1].spec.comparability.fixedCommandDigest
  ' "$file" "$other" >/dev/null || { echo "workload intake records are not comparable" >&2; exit 1; }
  echo "comparable workload intake: $(basename "$file") and $(basename "$other") (offline)"
else
  echo "valid workload intake: $(basename "$file") (offline, no network calls)"
fi
