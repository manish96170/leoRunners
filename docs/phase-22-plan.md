# Phase 22 Plan

Phase 22 turns the controller's observability contracts into an operator-facing
AWS runbook. CloudWatch is a visibility and notification layer; lifecycle
state, leases, and cleanup remain authoritative in the controller's durable
state store. Alarms must not terminate runners, mutate capacity, or make
security decisions automatically.

## Objectives

- Route structured controller logs and Embedded Metric Format (EMF) records to
  an encrypted, retained CloudWatch Log Group.
- Publish the existing low-cardinality lifecycle metrics under a stable
  `LeoRunners/Controller` namespace.
- Provide actionable alarms for controller availability, provisioning failure,
  reconciliation failure, stale runners, error rate, and lifecycle p99.
- Provide one operational dashboard that starts with alarm status, then shows
  lifecycle throughput, active runners, failures, and tail latency.
- Document triage, missing-data interpretation, cardinality controls,
  escalation, rollback, and read-only validation.

## CloudWatch contract

The controller emits logs and events with correlation identifiers for
investigation. Metric dimensions are an allowlist, not a copy of event fields.
The allowed dimensions are `provider`, `region`, `outcome`, `event_type`,
`state`, `capacity_owner`, `capacity_pool`, and `error_class`; never add job,
run, repository, runner, request, token, or secret values as dimensions.

The baseline metric families are:

- `lifecycle_events_total`: counter by finite event type and provider.
- `lifecycle_duration_seconds`: histogram by finite phase and provider; use
  p99 for alerting and investigation.
- `active_runners`: gauge by provider.

The deployment layer may add derived metrics, but every new metric needs an
owner, unit, retention decision, bounded dimensions, and a documented alert
rationale. EMF records should be written to the controller log stream and use
a fixed namespace, service, and environment dimension set.

## Alarm policy

Use one-minute periods for fast operational signals and an M-of-N evaluation,
normally two of three datapoints. Configure `TreatMissingData` explicitly:

- `notBreaching` for error counters and duration metrics when no workload is
  expected; no traffic must not page as an error.
- `breaching` for a controller heartbeat or readiness signal; silence is the
  failure condition.
- `missing` only when `INSUFFICIENT_DATA` is itself an intentional operator
  signal. Do not rely on the CloudWatch default.

Latency alarms must use an extended statistic such as `p99`; an average can
hide a broken tail. Thresholds are service-level objectives to be calibrated
from at least one normal operating window, then reviewed after every image,
provider, or capacity change.

Alarm actions should notify the owning on-call channel or create an incident.
They should not invoke EC2 stop/terminate, Auto Scaling, Lambda remediation,
or controller lifecycle APIs. A composite alarm may group related signals and
deployment suppression may be used during an approved maintenance window.

## Dashboard layout

Use an eight-hour default window and inherited widget periods. Keep the first
row for alarm status. Follow with number widgets for active runners and
failure rate, line graphs for lifecycle throughput and provisioning duration,
a full-width p99 latency graph, and a Logs Insights table for correlated
failures. Use a dashboard variable for environment or provider only when its
values are bounded and operationally useful. Do not create one dashboard per
repository, job, run, or runner.

## Delivery sequence

1. Review the metric and log contract against the running image and deployment
   environment.
2. Create the log group, retention, encryption, alarms, and dashboard in the
   target account and Region with change review.
3. Deploy with alert actions disabled or routed to a test destination.
4. Generate a signed synthetic webhook and one disposable validation job.
5. Confirm log correlation, metric ingestion, alarm transitions, dashboard
   panels, and cleanup before enabling on-call notifications.
6. Record the baseline p99, normal active-runner range, and expected event rate
   in the service record.

## Exit criteria

- Logs are retained according to the approved policy and contain no secrets.
- Metrics remain within the dimension and series budget during a representative
  workload test.
- Every alarm has a runbook link, owner, explicit missing-data treatment, and
  tested notification route.
- The dashboard supports provider, region, and environment investigation
  without high-cardinality filters.
- Read-only validation passes, and a rollback has been rehearsed without
  leaving an untracked runner, lease, or credential.
