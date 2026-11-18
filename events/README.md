# Lifecycle Events v1

`schema.v1.json` defines the append-only event contract emitted by the
controller and its trusted adapters. The examples are offline fixtures; they
do not authorize cloud calls, GitHub calls, scheduling, or cleanup.

## Contract

Every event has a stable `eventId`, an ordered `sequence`, an explicit
`eventType`, tenant/job identity, a source component, a deduplication key, a
replay key, an attempt number, and a finite retention expiry. `occurredAt`
is the event time; consumers must not use arrival time to infer lifecycle
order. `data.status` is the normalized state. Secrets, tokens, raw logs,
credentials, and unbounded payloads are outside this contract.

Events are immutable. Corrections are new events with a new `eventId` and a
causation reference; existing records are never edited in place. Producers
must reuse the same deduplication key when retrying the same logical event.
Consumers should persist the key before acknowledging work, make handlers
idempotent, and tolerate duplicate delivery and out-of-order replay.

## Evolution

`apiVersion` is versioned independently from event type. Additive optional
fields may be added within a version. Consumers must ignore unknown fields and
must validate only fields they own. A breaking change requires a new API
version, compatibility fixtures, and a migration window. Event type names are
not reused for different meanings.

The `data.extensions` object is an intentionally bounded namespace for
non-critical observations. Extension keys use lowercase names and values are
scalar. Extension consumers may annotate, index, export, or alert on events;
they must not mutate the event, issue credentials, schedule work, assign jobs,
provision or terminate runners, or change security policy. No critical
lifecycle control may depend on an extension consumer being available.

## Retention and replay

Retention is explicit per event. Store only the minimum redacted metadata and
delete or render records inaccessible after `retention.expiresAt`. Replay is
read-only: select a bounded time range or event-id set, preserve original
`eventId` and `occurredAt`, and attach a new consumer attempt outside the
event payload. Replay must not re-run lifecycle side effects.

## Backpressure and delivery

The lifecycle producer must remain bounded and must not block job execution
forever on an extension consumer. Use a finite queue, measurable drop or
retry counters, bounded retries, and an explicit dead-letter path for events
that cannot be delivered. A full extension queue may degrade telemetry, but it
must not prevent cancellation, cleanup, or termination. Consumers should apply
backoff and honor shutdown deadlines.

## Offline validation

Run:

```sh
./events/validate.sh
```

The validator uses only local JSON parsing and checks schema-required fields,
event-specific status rules, identity consistency, deduplication/replay keys,
retention, bounded extensions, and forbidden secret-shaped content. It also
rejects duplicate event IDs or deduplication keys across the supplied fixtures.

## Sink operations v1

`sink-contract.v1.json` defines the offline operational contract for the
append-only event sink and archive. The safe fixture requires local JSONL
delivery, persist-before-ack deduplication, bounded retries and queue drops,
owner-only `0600` archive permissions, metadata-only redaction, and bounded
read-only replay. `sink-behavior.v1.json` records deterministic evidence for
duplicate suppression, event-ID-preserving replay, and measurable overflow that
does not block cleanup.

Validate these declarations without contacting an external sink:

```sh
./tools/event-sink-validation/test.sh
```

The validator rejects external transport declarations, raw payload handling,
permissive archive modes, replay side effects, and inconsistent drop evidence.
