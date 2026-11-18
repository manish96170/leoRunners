# Intelligence Evidence and Configuration

This directory contains versioned, offline-validated examples for optional
failure analysis. Intelligence is advisory around the runner control plane;
it is never required to schedule, execute, or clean up a job.

## Files

- `config-disabled.v1.yaml` is the default-safe profile with no model calls.
- `config-local.v1.yaml` opts into a customer-controlled local model.
- `config-hosted.v1.yaml` opts into a hosted provider with explicit handling.
- `failure-event-*.v1.json` are synthetic events with redacted log context.
- `schema.v1.json` defines the stable document envelope.
- `validate.sh` validates supplied files without network access.

Run the offline checks:

```sh
./intelligence/validate.sh
```

## Privacy and evidence boundaries

1. Analysis is opt-in per tenant, policy, and trigger. `enabled: false` means
   no model endpoint may be contacted.
2. Redaction happens before persistence or provider transmission. Evidence may
   contain command names, exit codes, phase names, and short sanitized excerpts,
   but never tokens, credentials, cookies, authorization headers, source code,
   customer payloads, complete environment values, or unbounded raw logs.
3. The controller stores event metadata and a redacted evidence reference. A
   provider receives only the explicitly allowed fields, never the workspace.
4. Local mode keeps inference and evidence inside the customer boundary.
   Hosted mode requires recorded consent scope, region, provider, and retention.

## Retention

Retention is declared in finite, non-negative hours. Event metadata and
redacted excerpts have independent limits so operational records can outlive
sensitive evidence. Deletion is due at `created_at + retention`; expired
evidence must be removed or cryptographically rendered inaccessible.

## Opt-in triggers

Supported triggers are `job_failed`, `job_timed_out`, and `repeated_failure`.
They are explicit allowlists, not an implicit subscription to every job. A
trigger produces a recommendation only after redaction and tenant approval.
Cancellation and successful jobs do not invoke analysis in these profiles.

## Provider replacement

The controller depends on a narrow contract: analyze redacted input, return a
bounded structured result, and identify the provider/model used. Changing
`provider.id` is an explicit configuration replacement, not a silent fallback.
The replacement must preserve the input allowlist, output schema, timeout,
audit fields, and retention guarantees. An unavailable provider fails closed
to “no analysis” and cannot affect the job lifecycle.

## Deterministic action policy

Model output is untrusted advisory data. The platform accepts only the fixed
action codes `retry`, `change_capacity`, `inspect_image`, `open_incident`, and
`none`, with bounded confidence and evidence. Actions are proposed, never
autonomously applied to customer workloads. The model cannot terminate
runners, change IAM/networking, read secrets, or bypass approval. Identical
event input and policy version must yield the same action decision regardless
of provider wording.

The examples are fixtures, not permission to contact a model provider or retain
production evidence.
