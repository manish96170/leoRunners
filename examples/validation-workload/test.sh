#!/usr/bin/env bash
# Static checks that keep the sample safe to run: it must stay pull-request-only,
# read-only, secret-free and pinned. Needs no network. Uses actionlint and
# terraform when they are installed.
set -euo pipefail
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$here"
wf=.github/workflows/validate.yml
fail() { printf 'validation-workload test FAILED: %s\n' "$*" >&2; exit 1; }

bash -n scripts/report-environment.sh scripts/terraform-environment.sh

# Triggers: pull_request into main only.
python3 - "$wf" <<'PY' || exit 1
import re, sys
text = open(sys.argv[1]).read()
on = re.search(r"(?ms)^on:\n(.*?)^\S", text + "\nx").group(1)
triggers = re.findall(r"^  ([a-z_]+):", on, re.M)
if triggers != ["pull_request"]:
    sys.exit(f"workflow triggers must be exactly pull_request, found {triggers}")
if "branches: [main]" not in on:
    sys.exit("pull_request must be limited to main")
PY

python3 - "$wf" <<'PY' || exit 1
import re, sys
t = open(sys.argv[1]).read()
m = re.search(r"(?m)^permissions:\n((?:  [^\n]*\n)+)", t)
if not m or m.group(1).strip() != "contents: read":
    sys.exit("top-level permissions must be exactly contents: read")
if re.search(r"(?m)^\s+(id-token|contents|packages|pull-requests|actions):\s*write", t):
    sys.exit("no write permission may be requested")
PY

if grep -nE '\$\{\{\s*secrets\.' "$wf"; then fail "the workflow must not reference secrets"; fi
if grep -nE 'workflow_dispatch|schedule:|pull_request_target|push:' "$wf"; then fail "forbidden trigger"; fi

# Every action is pinned to a full commit SHA.
while IFS= read -r line; do
  ref=${line##*uses: }; ref=${ref%% *}
  case "$ref" in ./*) continue ;; esac
  [[ "$ref" =~ @[0-9a-f]{40}$ ]] || fail "action not pinned to a full SHA: $ref"
done < <(grep -E '^\s+(- )?uses:' "$wf")

# Same-repository pull requests only, on every job.
jobs=$(grep -cE '^  [a-z][a-z-]*:$' <(sed -n '/^jobs:/,$p' "$wf") || true)
guards=$(grep -c "head.repo.full_name == github.repository" "$wf" || true)
[ "$jobs" -eq "$guards" ] || fail "every job needs the same-repository guard ($jobs jobs, $guards guards)"

# Pinned tool versions carry checksums.
. ./versions.env
for v in TERRAFORM_SHA256_LINUX_AMD64 ACTIONLINT_SHA256_LINUX_AMD64 ACTIONLINT_SHA256_LINUX_ARM64; do
  [[ "${!v}" =~ ^[0-9a-f]{64}$ ]] || fail "$v is not a SHA-256"
done
[[ "$NODE_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "NODE_VERSION must be exact"
[[ "$TERRAFORM_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "TERRAFORM_VERSION must be exact"

[ -s yarn.lock ] || fail "yarn.lock is missing"
grep -q '"packageManager"' package.json && fail "packageManager would force corepack, which Node 25+ no longer ships"

if command -v actionlint >/dev/null 2>&1; then actionlint "$wf"; else echo "actionlint not installed; skipped" >&2; fi
if command -v terraform >/dev/null 2>&1; then terraform fmt -check -recursive terraform; else echo "terraform not installed; skipped" >&2; fi

printf '%s\n' 'validation-workload tests passed'
