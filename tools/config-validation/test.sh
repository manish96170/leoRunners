#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
"$ROOT/tools/config-validation/validate.sh" "$ROOT"
python3 - "$ROOT/tools/config-validation/fixtures" <<'PY'
import json
import pathlib
import sys

fixtures = pathlib.Path(sys.argv[1])
safe = json.loads((fixtures / "config-safe.v1.json").read_text())
unsafe = json.loads((fixtures / "config-unsafe.v1.json").read_text())
assert safe["diagnostics"] == {"containsSecretValues": False, "persistsSecretValues": False}
assert all(not safe["features"][name] for name in ("githubJIT", "ai", "extensions"))
assert unsafe["diagnostics"]["containsSecretValues"] is True
assert unsafe["diagnostics"]["persistsSecretValues"] is True
print("PASS: configuration fixtures")
PY
