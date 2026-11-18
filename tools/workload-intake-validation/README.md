# Workload intake validation

This offline validator checks the Phase 60 workload-intake contract. It makes
no GitHub, cloud, package-manager, credential, or network calls.

The contract requires:

- a repository URL, immutable 40-character commit, workflow path, and digest;
- a workload manifest digest and a SHA-256 digest of the exact bounded command;
- provenance, submitter, and non-empty approval metadata with an expiry;
- exact tool versions rather than floating versions or ranges;
- cache, network, and security observations with safe isolation defaults;
- redaction and raw-payload exclusion markers;
- fixed commit, manifest, and command identities for comparison.

Run the focused suite:

    ./tools/workload-intake-validation/test.sh

Validate one intake, or two intakes for fixed-identity comparability:

    ./tools/workload-intake-validation/validate.sh
    ./tools/workload-intake-validation/validate.sh first.json second.json

Exit code `0` means valid, `1` means rejected or incomparable, and `2` means
the input or required offline tooling is missing or malformed.
