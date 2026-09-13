# Observability Operations

This runbook covers the AWS operational view of Leo Runners. CloudWatch is
diagnostic and alerting infrastructure. The controller's durable lifecycle
state is authoritative for whether a runner exists, is assigned, or requires
cleanup.

## Signals

CloudWatch Logs receives structured controller events and EMF metric records.
Search with the correlation ID, provider, region, event type, and error class.
Do not paste tokens, full webhook payloads, repository secrets, or bootstrap
contents into tickets or queries.

The core metrics are:

| Metric | Meaning | Useful statistic |
| --- | --- | --- |
| `lifecycle_events_total` | Lifecycle event volume | Sum, rate |
| `lifecycle_duration_seconds` | Time spent in a lifecycle phase | p50, p95, p99 |
| `active_runners` | Current runner count | Maximum, latest |

Use only bounded dimensions: `provider`, `region`, `outcome`, `event_type`,
`state`, `capacity_owner`, `capacity_pool`, and `error_class`. A job ID or
runner ID belongs in a log field, never a metric dimension.

## First response

1. Open the alarm and note account, Region, metric dimensions, first breach,
   last OK time, and missing-data treatment.
2. Check `GET /healthz` and `GET /metrics` through the private service path.
3. Open the dashboard for the same Region and environment. Compare active
   runners, lifecycle event rate, provisioning failures, and p99 duration.
4. Query logs using the alarm window and correlation fields. Determine whether
   the fault is controller, GitHub, provider, network, state store, image, or
   workload-specific.
5. Check durable state and reconciliation results before retrying or deleting
   anything. Never infer cloud-resource ownership from a missing log event.
6. Record the incident timeline, scope, action taken, and cleanup evidence.

## Triage by symptom

### Controller unavailable or heartbeat alarm

Check container or process restart count, readiness, state-store errors, and
recent configuration or image changes. Confirm that the process has stopped
accepting new work before restarting it. Restore one instance, wait for a
reconciliation pass, and verify that leases and provider resources converge.
Escalate immediately if durable state cannot be read or writes are failing.

### Provisioning failures or provider throttling

Group by provider, Region, capacity pool, and error class. Compare failures
with `lifecycle_duration_seconds` p99 and provider API health. Check capacity,
quotas, launch/template configuration, network reachability, and IAM role
assumption. Pause new workload intake if retries could amplify provider load;
allow bounded cleanup to continue.

### Stale runner or cleanup alarm

Find the runner in durable state by its correlation and runner identifiers,
then inspect the last lifecycle transition and reconciliation attempt. Verify
whether the cloud resource and GitHub registration still exist. Use the
controller's idempotent cleanup path and confirm termination plus registration
removal. Escalate as a data-integrity incident when a resource is untracked,
or when cleanup cannot be proven.

### High p99 lifecycle duration

Use p99, not average, to identify tail behavior. Split by phase, provider,
Region, capacity owner, and error class. Compare p50 versus p99, event volume,
retry counts, provider latency, GitHub API latency, state-store latency, and
active capacity. A high p99 with a normal p50 usually indicates a dependency
tail, throttling, queue starvation, or a small unhealthy pool. Do not increase
timeouts or capacity until the phase and dependency are identified.

### Missing data or `INSUFFICIENT_DATA`

First determine whether the service is expected to be emitting. Check process
health, log-group ingestion, EMF JSON validity, metric namespace, account,
Region, and clock skew. For error and duration metrics, no workload may
legitimately produce no datapoints; treat that as `notBreaching` and use a
separate heartbeat/readiness alarm. For heartbeat metrics, silence should be
`breaching`. Resolve a missing-data alarm as an observability incident until
the ingestion path is proven healthy.

## Cardinality controls

Keep metric dimensions finite and review any new value vocabulary. Never use
repository, workflow, run, job, attempt, runner, request, correlation ID,
URL, token, commit SHA, or arbitrary exception text as a dimension. Put those
values in redacted structured logs and use exact log queries for investigation.

Watch for sudden series growth after a deployment. If the exporter rejects a
series or the CloudWatch cost/series budget grows unexpectedly, disable the
new metric or route it to a non-paging diagnostic sink, then roll back the
producer. Do not solve cardinality by truncating identifiers into collisions.

## Escalation

Page the controller owner for sustained health, reconciliation, state-store,
or cleanup failures. Page the cloud/platform owner for provider throttling,
quota, network, IAM, or regional capacity failures. Page the GitHub
integration owner for JIT token, registration, webhook, or API failures.
Page security immediately for credential exposure, untrusted workload escape,
unexpected privilege, or an instance that cannot be attributed and cleaned
up. Include Region, environment, impact, first-seen time, alarm ARN, bounded
dimensions, correlation IDs, and the last known safe state. Redact secrets.

## Rollback and recovery

1. Stop promotion and suspend new workload intake if the change can create
   resources, duplicate runners, or retry storms.
2. Preserve logs, alarm history, durable state snapshots, and the deployment
   version for the incident record.
3. Roll back the controller image or configuration through the approved
   deployment mechanism. Do not manually edit state or terminate resources
   based only on a dashboard panel.
4. Run reconciliation and verify leases, cloud instances, GitHub registrations,
   and state records agree. Use idempotent cleanup for confirmed orphans.
5. Re-enable intake gradually after health, event ingestion, p99, and cleanup
   are stable for the agreed observation window.

Changing an alarm threshold or notification target is not a controller
rollback. Record it as an observability configuration change and restore the
previous alarm definition after the incident.

## Read-only validation

From the repository root, run the offline checks before any live validation:

```sh
(cd controller && go test ./...)
(cd controller && go test -race ./...)
(cd controller && go vet ./...)
(cd controller && go build ./...)
sh tools/smoke
sh tools/cloud-validation/validate.sh --provider aws
```

The cloud validation command is read-only by default. A live disposable test
requires an explicit approval workflow and the confirmation string documented
by `tools/cloud-validation/validate.sh`; never put credentials in shell
history or command arguments. For an AWS read-only inspection, use the target
account and Region explicitly:

```sh
aws sts get-caller-identity
aws logs describe-log-groups --log-group-name-prefix /leo-runners/ --region "$AWS_REGION"
aws cloudwatch describe-alarms --state-value ALARM --region "$AWS_REGION"
aws cloudwatch list-dashboards --region "$AWS_REGION"
```

Validate one signed synthetic webhook and one disposable job only after the
read-only checks pass. Confirm the complete path: log event, EMF metric,
dashboard panel, alarm test notification, durable state transition, GitHub
registration, and cloud-resource cleanup. Save a redacted report.

## Completion checklist

- [ ] Alarm owner, runbook link, threshold, period, M-of-N, and missing-data
      policy are recorded.
- [ ] Dashboard uses an eight-hour view, alarm status, lifecycle throughput,
      active runners, failures, and p99 latency.
- [ ] Log retention and encryption match the environment policy.
- [ ] No metric or log field exposes credentials or unbounded identifiers.
- [ ] A missing-data drill and a cleanup/orphan drill were completed.
- [ ] Escalation contacts and rollback version are recorded.
- [ ] Validation output and cleanup evidence are attached to the release record.
