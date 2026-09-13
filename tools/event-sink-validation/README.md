# Event sink validation

`validate.sh` is an offline contract and evidence validator for the Phase 41
event sink. It performs no network, cloud, archive, or external sink calls.

It validates the versioned `EventSinkContract` for local append-only delivery,
persist-before-ack idempotency, bounded queue/drop behavior, owner-only `0600`
archive permissions, metadata-only redaction, bounded retention, and read-only
replay. The behavior fixture proves duplicate delivery suppression, event ID
preservation during replay, and measurable queue overflow without blocking
cleanup.

Run the focused suite:

```sh
./tools/event-sink-validation/test.sh
```

Unsafe fixtures must fail. This tool validates declarations and deterministic
evidence only; it does not claim that a production archive or sink was reached.
