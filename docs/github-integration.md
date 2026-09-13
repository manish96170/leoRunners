# GitHub Integration

The primary event is `workflow_job`, normalized from queued, in-progress, completed, and cancelled actions. Webhook delivery is authenticated, deduplicated, persisted, and acknowledged quickly; the worker performs long-running lifecycle work.

The current GitHub REST API exposes repository-level JIT runner configuration at `POST /repos/{owner}/{repo}/actions/runners/generate-jitconfig`. It returns an encoded configuration for startup and requires repository administration permission. Prefer GitHub App installation access tokens with the smallest required scope. Registration tokens are an alternative but expire after one hour.

The controller must tolerate duplicate and missing events, API rate limits, cancellation races, reruns, stale jobs, runner registration failure, and controller restarts. GitHub remains the source of truth for job status; our state records the orchestration lease and provider correlation.

Phase 3 adds an injected JIT client and registration-verifier boundary. The encoded JIT value is transient bootstrap input and is excluded from durable runner records and lifecycle-event data. The real GitHub client calls the repository JIT endpoint with a short-lived bearer token; the runner bootstrap invokes the pre-baked runner's JIT entrypoint and removes temporary sensitive material on exit.

References: https://docs.github.com/en/rest/actions/self-hosted-runners and https://docs.github.com/en/webhooks/webhook-events-and-payloads#workflow_job.
