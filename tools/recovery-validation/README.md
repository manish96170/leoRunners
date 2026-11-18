# Recovery validation

This offline Phase 53 harness validates deterministic disaster-recovery
evidence for the controller state and coordination contracts. It checks:

- file snapshot recovery from a corrupt primary and valid backup;
- DynamoDB-style conditional revision protection without overwrite;
- event replay with duplicate suppression and stable event IDs;
- monotonic lease fencing and stale-write rejection;
- complete, idempotent stale-runner reaping;
- two-replica takeover without duplicate assignment or capacity loss;
- a bounded recovery window;
- owner-only `0600`, atomic, redacted evidence with zero cloud calls.

The harness reads JSON fixtures only. It makes no cloud calls, filesystem
mutations, controller calls, or destructive operations.

Run the focused suite:

```sh
./tools/recovery-validation/test.sh
```

Validate a specific evidence fixture:

```sh
./tools/recovery-validation/validate.sh validation/recovery/recovery-evidence.v1.json
```
