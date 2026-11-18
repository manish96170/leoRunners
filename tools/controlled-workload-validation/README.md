# Controlled workload validation

This offline validator checks the Phase 61 execution contract. It never runs
the requested commands and makes no GitHub, cloud, package-manager, or network
calls.

The contract requires:

- a clean checkout at a lowercase, full 40-character repository commit;
- an explicit executable allowlist and bounded argv for every command;
- deterministic command digests, bounded timeout/output/disk/process limits;
- environment names without values, with secret exclusion and redaction markers;
- network mode `none` with disabled egress by default, or an explicit approved
  egress reference;
- toolchain and manifest provenance plus cleanup requirements;
- evidence bound to the requested commit, bounded measurements, complete
  cleanup, and redacted output.

Shell metacharacters, loops and other unbounded command forms, floating refs,
secret-shaped values, unsafe network defaults, missing cleanup, and digest
mismatches are rejected before any execution could be considered.

Run the focused suite:

    ./tools/controlled-workload-validation/test.sh

Validate one record:

    ./tools/controlled-workload-validation/validate.sh

Exit code `0` means valid, `1` means rejected, and `2` means malformed input
or missing offline tooling.
