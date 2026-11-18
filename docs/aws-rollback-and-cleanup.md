# Rollback and Request-Scoped Cleanup

## Ownership tags

Put identifying tags on everything through the modules' `tags` input. The
recommended convention is:

```text
Environment=<name>   Owner=<team>   RequestId=<approved request ID>   CleanupDeadline=<UTC timestamp>
```

Runner instances additionally carry the fixed contract tags
`Platform=ci-runner` and `ManagedBy=leo-runners` (and per-job tags added by the
controller). The `aws-ephemeral-runner` module publishes the exact tag
intersection as `cleanup_selector`.

## Rolling back the controller

Remove capacity in the reverse order it was added, one reviewed change each.

1. **Stop admitting work.** Set `desired_count = 0`, or set `enable_service = false` and `enable_listener_rule = false` together. The service depends on the route, so Terraform removes the service first. Requests then reach the gateway's fixed default response (404 unless you changed it).
2. **Verify nothing is admitted.** GitHub's webhook delivery log for the app shows the failures. Confirm there are no running tasks and no registered targets.
3. **Handle in-flight work before removing state.** Review active runner records and instances (see below) before touching the state table.
4. **Remove the foundation** only after step 3, by setting `create_foundation = false`. Expect and review these safeguards:
   - the DynamoDB table has deletion protection; disable it in a separate reviewed change (`state_enable_deletion_protection = false`) first;
   - the ECR repository will not delete while it holds images;
   - secret containers enter their recovery window (`secret_recovery_window_in_days`, default 7) rather than vanishing.
5. **Prove it.** Discover by the request tag and require zero matches.

The gateway module is separate. Removing it deletes only the listener,
certificate attachments, and ingress rules it created; it never touches the
load balancer, certificate, or DNS.

## Request-scoped cleanup of runners

Select resources only by the full tag intersection, never by name, VPC,
subnet, or instance type:

```sh
aws ec2 describe-instances \
  --filters Name=tag:Platform,Values=ci-runner \
            Name=tag:ManagedBy,Values=leo-runners \
            Name=tag:RequestId,Values="$REQUEST_ID" \
            Name=instance-state-name,Values=pending,running,stopping,stopped \
  --query 'Reservations[].Instances[].InstanceId' --output text
```

Review the list against the approved request, terminate exactly those
instances, then repeat the query until it returns nothing. For other resource
types use `aws resourcegroupstaggingapi get-resources --tag-filters
Key=RequestId,Values="$REQUEST_ID"`; not every resource type is taggable, so
also check the Terraform state.

If `request_id` is not set, the contract defines no request-scoped selector and
nothing may be cleaned up by guessing.

## Fail closed

If zero remaining resources cannot be proven by the deadline, stop new
provisioning, keep a bounded redacted record of what was found and done, and
escalate to the named owner. Do not launch replacements and do not widen the
selector to make cleanup succeed.

The controller's own reaper terminates expired or orphaned runners by their
ownership tags and `ExpiresAt`, including after a restart. It is a backstop,
not a substitute for the proof above.
