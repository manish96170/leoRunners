# GitHub Runtime Contract

## What the controller reads

| Variable | Required | Source in `aws-ecs-controller` |
|---|---|---|
| `GITHUB_WEBHOOK_SECRET` | always | Secrets Manager reference |
| `GITHUB_APP_INSTALLATION_TOKEN` | only for JIT | Secrets Manager reference, injected when `github.jit_enabled` |
| `GITHUB_RUNNER_GROUP_ID` | with JIT | `github.runner_group_id` |
| `GITHUB_ALLOWED_ORGANIZATIONS` / `GITHUB_ALLOWED_REPOSITORIES` | with JIT, at least one | `github.allowed_organizations` / `allowed_repositories` |
| `GITHUB_ALLOWED_LABELS` | with JIT | `github.allowed_labels` |
| `GITHUB_ALLOW_FORKS`, `GITHUB_API_BASE_URL` | optional | `github.allow_forks`, `github.api_base_url` |

The module enforces the same rules the controller enforces at startup, so a
bad combination fails at plan time instead of as a crash loop. No GitHub App
ID, organization, repository, or runner label is a default anywhere in
LeoRunners.

## Finding: a durable JIT deployment needs token refresh

Confirmed from the source:

- `controller/cmd/runner-controller/main.go` builds the GitHub client with `github.StaticBearerToken(token)`, a token read once from `GITHUB_APP_INSTALLATION_TOKEN` at process start.
- ECS injects secrets only when a task starts; it never refreshes a running task.
- GitHub App installation access tokens expire after about one hour.

So with a static token, JIT stops working roughly an hour after the task
starts. The controller already has the right seam:
`github.BearerTokenSource` in `controller/internal/github/jit.go` supplies a
token per request, and `StaticBearerToken` is only one implementation.

## Options

1. **Generic GitHub App support in LeoRunners (recommended).** Add a token
   source that holds the App ID, installation ID, and private key, signs a
   short-lived JWT, requests an installation token, and caches it until shortly
   before expiry. The private key would be a Secrets Manager reference like any
   other secret. This is a controller code change and is not implemented yet;
   the variable and environment names are not final. The module will gain the
   matching secret container and environment mapping when it lands.
2. **External rotation (stop-gap only).** A customer-owned job mints an
   installation token and writes it to the token secret, then forces a new
   deployment before the hour is up. Because deployments stop the old task
   before starting the new one, every rotation is a short outage, so this suits
   short validation runs, not production.
3. **JIT off.** `github.jit_enabled = false` (default) runs the webhook
   receiver with a fake provider and provisions nothing real. Useful to verify
   the front door, signature checks, and state.

## Permissions

The repository-level JIT endpoint requires repository administration
permission (see `docs/github-integration.md`). Grant the GitHub App the
smallest permissions that satisfy the endpoint you use, install it only on the
organizations or repositories you list in the allowlists, and review the
App's permissions against GitHub's current documentation when you create it.
Creating the GitHub App is a customer action; nothing in this repository
creates one.

## Known runtime limits to review before production

- One controller replica. The coordination package exists but is not wired into the executable, so `desired_count` is limited to 0 or 1.
- The webhook handler runs the job's orchestration inside the HTTP request with a long context. GitHub expects a response within about 10 seconds and the ALB idle timeout defaults to 60 seconds. Verify queued-event handling latency against real provisioning before relying on it, and consider acknowledging quickly and working asynchronously.
- ALB to task traffic is HTTP inside the VPC, not end-to-end TLS.
