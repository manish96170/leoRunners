# Extensions Deployment

This runbook covers deploying optional Leo Runners extensions. An extension is
an isolated, advisory consumer of versioned events. Registration and tenant
enablement are configuration controls; they do not grant provider, IAM,
GitHub, credential, network, lifecycle, or cleanup authority.

## Before deployment

Review the extension manifest and confirm its stable ID, contract version,
owner, source or image digest, capabilities, runbook, queue and timeout
bounds, output limits, retention, and rollback target. Keep the global gate
disabled and every tenant allowlist empty while registration is reviewed.

Run the offline checks from the repository root:

```sh
(cd controller && go test ./...)
(cd controller && go test -race ./...)
(cd controller && go vet ./...)
(cd controller && go build ./...)
sh extensions/validate.sh
```

Confirm that validation rejects duplicate IDs, unsupported versions, unknown
fields, wildcard grants, secrets, unbounded dimensions, and forbidden
lifecycle, provider, security, or credential actions. Record the registry
snapshot, hash, configuration diff, approver, and deployment window.

## Disabled-by-default registration

Register the reviewed manifest without enabling event delivery. Verify:

- the global extension gate is disabled unless explicitly changed;
- the tenant allowlist contains no implicit or wildcard entries;
- observe-only mode is the default for a new rollout;
- no cloud resource, credential, provider client, or lifecycle action is
  created by registration; and
- the controller remains healthy with the extension absent.

Registration is successful only when the manifest is accepted and the
extension remains unsubscribed until both explicit gates are enabled.

## Tenant allowlist and observe-only rollout

1. Add the exact extension ID to one approved tenant's allowlist.
2. Keep the global policy in observe-only mode and record the effective time.
3. Send synthetic events for the approved tenant and for a disabled tenant.
4. Verify that only the approved tenant is delivered, and that the negative
   tenant test receives no extension event.
5. Confirm bounded queue, timeout, panic, drop, redaction, and advisory
   metrics. Inspect logs without exposing payloads or secrets.
6. Compare authoritative controller decisions with the extension disabled and
   enabled. They must be unchanged by advisory output.
7. Expand tenant scope only after the agreed observation window and owner
   approval.

Observe-only output may be stored as a recommendation or annotation. It must
not terminate a runner, mutate capacity, grant credentials, change security
policy, call a provider, or alter cleanup.

## Configuration review

Review the deployment values for global enablement, observe-only mode, exact
tenant allowlists, registry hash, digest, queue size, handler timeout, report
buffer, shutdown timeout, output limit, retention, metrics namespace, log
destination, environment, notification route, and validation mode. Missing
enablement values must remain disabled. Keep secrets in the deployment secret
mechanism and verify they are redacted from configuration output.

## Runtime registration versus live cloud validation

Runtime registration validation is a local or deployment-time contract check.
It verifies schema, identity, policy, tenant scope, limits, and advisory
boundaries. It must be safe to run without cloud credentials and must not
create or delete resources.

Live cloud validation is a separate, explicitly authorized operation using a
disposable AWS, GCP, or GitHub scope. Before starting it, confirm credentials,
region or project, resource limits, time bounds, cleanup behavior, and the
required confirmation flag. Keep its report separate from the registration
record and redact account IDs, tokens, URLs, and payloads as required.

The two results are independent: runtime validation does not prove cloud
access, and cloud validation does not enable an extension or expand its
authority. A live validation failure must not be “fixed” by weakening runtime
registration policy.

## Rollback

1. Remove the extension ID from affected tenant allowlists.
2. Disable the global extension gate if delivery continues or impact is
   uncertain.
3. Restore the last known-good registry, digest, and configuration snapshot.
4. Preserve metrics, redacted logs, advisory decisions, event IDs, and the
   configuration diff for the incident record.
5. Verify no subscription remains, then run a bounded synthetic event,
   reconciliation pass, and cleanup check.
6. Re-enable one tenant only after failure, timeout, drop, and telemetry
   signals are stable for the approved observation window.

Extensions must not perform cloud cleanup directly. Use the controller's
normal idempotent cleanup path for confirmed orphans, or the separately
approved live-validation cleanup procedure.

## Deployment checklist

- [ ] Manifest, digest, owner, limits, retention, and rollback target approved.
- [ ] Global registration remains disabled until the reviewed change.
- [ ] Tenant allowlist contains exact IDs only and has a negative test.
- [ ] Observe-only mode and synthetic-event evidence are recorded.
- [ ] Forbidden actions and secret/cardinality checks pass.
- [ ] Runtime registration and live cloud validation evidence are separate.
- [ ] Rollback was tested without affecting reconciliation or cleanup.
