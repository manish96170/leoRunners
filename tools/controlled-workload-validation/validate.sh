#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
schema="$root/controlled-workload/execution.schema.v1.json"
file=${1:-"$root/controlled-workload/execution.v1.json"}

command -v jq >/dev/null 2>&1 || { echo "validate.sh: jq is required" >&2; exit 2; }
command -v shasum >/dev/null 2>&1 || { echo "validate.sh: shasum is required" >&2; exit 2; }
[ -f "$schema" ] || { echo "schema not found: $schema" >&2; exit 2; }
[ -f "$file" ] || { echo "execution record not found: $file" >&2; exit 2; }
jq empty "$schema" >/dev/null || { echo "invalid schema JSON" >&2; exit 2; }
jq empty "$file" >/dev/null || { echo "invalid execution JSON: $file" >&2; exit 2; }

if ! jq -e '
  . as $root |
  .apiVersion == "controlled-workload.leorunners.io/v1" and
  .kind == "ControlledWorkloadExecution" and
  (.metadata.name | type == "string" and test("^[a-z][a-z0-9-]{2,63}$")) and
  (.metadata.version | type == "string" and test("^[0-9]+\\.[0-9]+\\.[0-9]+$")) and
  (.spec.request.repository.url | test("^https://github\\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")) and
  (.spec.request.repository.commit | test("^[0-9a-f]{40}$")) and
  (.spec.request.repository.cleanCheckout == true) and
  (.spec.request.allowlist.commands | type == "array" and length >= 1 and length <= 16 and all(.[]; test("^[a-zA-Z0-9][a-zA-Z0-9._+/-]{0,63}$")) and length == (unique | length)) and
  (.spec.request.commands | type == "array" and length >= 1 and length <= 8 and all(.[];
    (.id | test("^[a-z][a-z0-9-]{2,31}$")) and
    (.argv | type == "array" and length >= 1 and length <= 16 and all(.[]; type == "string" and length >= 1 and length <= 160 and (test("[\\r\\n;|&<>$`()!{}\\[\\]*?]") | not))) and
    (.cwd | test("^(?!/)(?!.*(?:^|/)\\.\\.(?:/|$))[A-Za-z0-9_.@/-]+$")) and
    (.timeoutSeconds | type == "number" and floor == . and . >= 1 and . <= 3600) and
    (.commandDigest | test("^sha256:[0-9a-f]{64}$")) and
    (.allowlisted == true) and
    (.argv[0] as $binary | ($root.spec.request.allowlist.commands | index($binary)) != null)
  ) and ([.[].id] | length == (unique | length))) and
  (.spec.request.limits.totalTimeoutSeconds | type == "number" and floor == . and . >= 1 and . <= 7200) and
  (.spec.request.limits.maxOutputBytes | type == "number" and floor == . and . >= 1024 and . <= 1073741824) and
  (.spec.request.limits.maxDiskBytes | type == "number" and floor == . and . >= 1048576 and . <= 10737418240) and
  (.spec.request.limits.maxProcesses | type == "number" and floor == . and . >= 1 and . <= 128) and
  (.spec.request.environment.variables | type == "array" and length <= 64 and all(.[]; test("^[A-Z][A-Z0-9_]{0,63}$")) and length == (unique | length)) and
  (.spec.request.environment.valuesRedacted == true) and
  (.spec.request.environment.secretsExcluded == true) and
  (.spec.request.network.mode | IN("none", "approved-egress")) and
  (.spec.request.network.enabled | type == "boolean") and
  (.spec.request.network.approved | type == "boolean") and
  ((.spec.request.network.mode == "none" and .spec.request.network.enabled == false and .spec.request.network.approved == false) or
   (.spec.request.network.mode == "approved-egress" and .spec.request.network.enabled == true and .spec.request.network.approved == true and (.spec.request.network.approvalRef | test("^req-[a-z0-9][a-z0-9-]{7,63}$")))) and
  (.spec.request.provenance.source | IN("controlled-intake", "offline-fixture")) and
  (.spec.request.provenance.manifestDigest | test("^sha256:[0-9a-f]{64}$")) and
  (.spec.request.provenance.toolchain | type == "array" and length >= 1 and length <= 16 and all(.[]; test("^[a-z0-9][a-z0-9._+-]{1,63}@[0-9]+\\.[0-9]+\\.[0-9]+$"))) and
  (.spec.request.provenance.requestedBy | test("^[a-z0-9][a-z0-9._-]{2,63}$")) and
  (.spec.request.provenance.approvalRef | test("^req-[a-z0-9][a-z0-9-]{7,63}$")) and
  (.spec.request.cleanup.required == true and .spec.request.cleanup.workspaceRemoval == true and .spec.request.cleanup.processReaping == true and .spec.request.cleanup.artifactRedaction == true) and
  (.spec.evidence.status | IN("complete", "failed", "blocked")) and
  (.spec.evidence.commit == .spec.request.repository.commit) and
  (.spec.evidence.startedAt | test("^20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")) and
  (.spec.evidence.finishedAt | test("^20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")) and
  (.spec.evidence.finishedAt >= .spec.evidence.startedAt) and
  (.spec.evidence.exitCode | type == "number" and floor == . and . >= 0 and . <= 255) and
  (.spec.evidence.outputBytes | type == "number" and floor == . and . >= 0 and . <= $limits.maxOutputBytes) and
  (.spec.evidence.diskBytes | type == "number" and floor == . and . >= 0 and . <= $limits.maxDiskBytes) and
  (.spec.evidence.outputDigest | test("^sha256:[0-9a-f]{64}$")) and
  (.spec.evidence.cleanup.status == "complete" and .spec.evidence.cleanup.workspaceRemoved == true and .spec.evidence.cleanup.processesReaped == true and .spec.evidence.cleanup.artifactsRedacted == true) and
  (.spec.evidence.redaction.status == "redacted" and .spec.evidence.redaction.environmentValuesExcluded == true and .spec.evidence.redaction.rawOutputExcluded == true and .spec.evidence.redaction.secretsScanned == true) and
  ((.spec.evidence.status != "complete") or (.spec.evidence.exitCode == 0)) and
  ((.spec.evidence.status == "complete") or (.spec.evidence.exitCode != 0)) and
  ((tostring | test("(?i)(AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|bearer[[:space:]]+[A-Za-z0-9._-]+|password[=:][^[:space:]]+|secret[=:][^[:space:]]+|token[=:][^[:space:]]+|private[ _-]?key|BEGIN[[:space:]]+(RSA |EC |OPENSSH )?PRIVATE KEY)") | not))
  ' --argjson limits "$(jq -c '.spec.request.limits' "$file")" "$file" >/dev/null; then
  echo "controlled workload validation failed: $file" >&2
  exit 1
fi

command_count=$(jq '.spec.request.commands | length' "$file")
i=0
while [ "$i" -lt "$command_count" ]; do
  argv=$(jq -c ".spec.request.commands[$i].argv" "$file")
  digest=$(printf '%s' "$argv" | shasum -a 256 | awk '{print "sha256:" $1}')
  expected=$(jq -r ".spec.request.commands[$i].commandDigest" "$file")
  [ "$digest" = "$expected" ] || { echo "command digest mismatch at index $i: $file" >&2; exit 1; }
  i=$((i + 1))
done

if jq -e 'any(.spec.request.commands[]; any(.argv[]; test("(^|[[:space:];])(while|until|watch|yes)([[:space:]]|$)|tail[[:space:]]+-f|(^|[[:space:]])for[[:space:]].*in([[:space:]]|$)")))' "$file" >/dev/null; then
  echo "unbounded command rejected: $file" >&2
  exit 1
fi

echo "valid controlled workload execution: $(basename "$file") (offline, no execution or cloud calls)"
