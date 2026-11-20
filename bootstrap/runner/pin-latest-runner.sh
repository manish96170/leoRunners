#!/bin/sh
# Prints the latest actions/runner release version and its published SHA-256
# checksums in the form used by user-data.sh. Review the output and update the
# pinned values there; this script changes nothing.
set -eu

command -v curl >/dev/null 2>&1 || { echo 'curl is required' >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo 'python3 is required' >&2; exit 2; }

curl -fsSL https://api.github.com/repos/actions/runner/releases/latest | python3 -c '
import json, re, sys
release = json.load(sys.stdin)
version = release["tag_name"].lstrip("v")
digests = dict(re.findall(r"<!-- BEGIN SHA ([a-z0-9-]+) -->([0-9a-f]{64})<!-- END SHA", release.get("body", "")))
for key in ("linux-x64", "linux-arm64"):
    if key not in digests:
        sys.exit("no published checksum for " + key + " in the release notes; check the release by hand")
print("RUNNER_VERSION=\"%s\"" % version)
print("RUNNER_SHA256_X64=\"%s\"" % digests["linux-x64"])
print("RUNNER_SHA256_ARM64=\"%s\"" % digests["linux-arm64"])
print("# published " + release.get("published_at", "unknown"), file=sys.stderr)
'
