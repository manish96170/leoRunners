# GitHub Runtime Contract

## What the controller reads

| Variable | Required | Source in `aws-ecs-controller` |
|---|---|---|
| `GITHUB_WEBHOOK_SECRET` | always | Secrets Manager reference |
| `GITHUB_APP_ID` | GitHub App mode | `github.app.app_id` (plain config) |
| `GITHUB_APP_INSTALLATION_ID` | GitHub App mode | `github.app.installation_id` (plain config) |
| `GITHUB_APP_PRIVATE_KEY` | GitHub App mode | Secrets Manager reference holding the PEM key |
| `GITHUB_APP_TOKEN_PERMISSIONS` | optional, App mode | `github.app.token_permissions`, for example `administration=write` |
| `GITHUB_APP_INSTALLATION_TOKEN` | static-token mode | Secrets Manager reference |
| `GITHUB_RUNNER_GROUP_ID` | with JIT | `github.runner_group_id` |
| `GITHUB_RUNNER_SCOPE` | optional, `repository` (default) or `organization` | `github.runner_scope` |
| `GITHUB_ALLOWED_ORGANIZATIONS` / `GITHUB_ALLOWED_REPOSITORIES` | with JIT, at least one | `github.allowed_organizations` / `allowed_repositories` |
| `GITHUB_ALLOWED_LABELS` | with JIT | `github.allowed_labels` |
| `GITHUB_ALLOW_FORKS`, `GITHUB_API_BASE_URL` | optional | `github.allow_forks`, `github.api_base_url` |

The controller and the Terraform module enforce the same rules, so a bad
combination fails at plan time or at startup instead of as a crash loop. No
GitHub App ID, installation, organization, repository, or runner label is a
default anywhere in LeoRunners.

## Authentication modes

JIT needs the controller to call GitHub's API. There are two mutually exclusive
ways to authenticate, and the controller refuses to start with both.

### GitHub App (recommended)

The deployer creates and owns a GitHub App. The controller holds the App's ID,
an installation ID, and the App's private key, and mints its own short-lived
installation tokens:

1. It signs a JWT (RS256) with the private key: `iat` 60 seconds in the past to
   allow for clock drift, `exp` 9 minutes ahead (GitHub allows at most 10), and
   `iss` set to the App ID.
2. It calls `POST /app/installations/{installation_id}/access_tokens` with the
   JWT. GitHub returns an installation token that expires after about an hour.
3. It caches the token and requests a new one 5 minutes before expiry. If a
   refresh fails while the cached token is still valid, the cached token keeps
   being used, so a short GitHub outage does not stop runners.
4. Concurrent callers share one refresh.

No redeploy is needed for token expiry. The private key is read once at start
from the deployer's own Secrets Manager secret. It is never logged, never
written to disk (the container root filesystem is read-only), and never
included in an error message. A key that is missing, malformed, passphrase
protected, not RSA, or smaller than 2048 bits stops the controller at startup.
A wrong App ID or installation ID is reported in the startup log by a background
check without blocking startup, and is retried for each job.

`GITHUB_APP_TOKEN_PERMISSIONS` (`name=level`, comma separated) optionally asks
GitHub for tokens narrower than the App's full grant. The token can never exceed
what the App was granted.

In App mode `GITHUB_API_BASE_URL` must be HTTPS, because the signed JWT is sent
to it.

### Static installation token

`GITHUB_APP_INSTALLATION_TOKEN` is read once at start and never refreshed. The
token expires after about an hour, and ECS or Kubernetes injects secrets only
when a task or pod starts, so rotating it means redeploying. Use it for short
validation runs only, or if an external system you already operate mints the
token and restarts the controller on rotation.

## Creating the GitHub App (a customer action)

Nothing in LeoRunners creates the App. In the account that owns the
repositories:

1. **Permissions.** Repository permissions: **Actions: read** (GitHub requires
   at least this to subscribe to `workflow_job` events). Metadata read is
   granted automatically. Then, depending on `GITHUB_RUNNER_SCOPE`:
   organization scope (preferred) needs the organization permission
   **Self-hosted runners: read and write**; repository scope needs the broad
   repository permission **Administration: read and write**. Grant nothing else.
2. **Webhook.** Subscribe to **Workflow job**. Set the URL to your gateway's
   webhook path and the secret to the value you store in the webhook secret
   container.
3. **Install** the App only on the repositories (or organization) you list in the
   controller's allowlists.
4. **Generate a private key** and store the PEM in the `github_app_private_key_secret_arn`
   secret. Do not put it in Terraform, tfvars, a repository, or a ticket.
5. **Installation ID.** GitHub includes it in the `installation` object of the
   webhook deliveries the App receives, and `GET /app/installations`
   (authenticated with an App JWT) lists it.
6. **Runner group.** Create or choose the runner group the runners register to
   and set its numeric ID as `github.runner_group_id`.

Storing the key, for example:

```sh
umask 077
# key.pem is the file GitHub gave you
aws secretsmanager put-secret-value \
  --secret-id "$(terraform output -raw github_app_private_key_secret_arn)" \
  --secret-string file://key.pem
rm -f key.pem
```

## Rotating the private key

GitHub lets an App hold more than one key. Add a new key in GitHub, store it in
the secret, force a new deployment (`aws ecs update-service --force-new-deployment`),
confirm the startup log reports `GitHub App authentication ok`, then delete the
old key in GitHub.

## Where runners register: repository or organization

`GITHUB_RUNNER_SCOPE` (`github.runner_scope` in Terraform) chooses the GitHub
endpoint the controller uses to create and look up runners.

| | `repository` (default) | `organization` |
|---|---|---|
| Creates runners with | `POST /repos/{owner}/{repo}/actions/runners/generate-jitconfig` | `POST /orgs/{org}/actions/runners/generate-jitconfig` |
| Looks runners up with | `GET /repos/{owner}/{repo}/actions/runners` | `GET /orgs/{org}/actions/runners` |
| App permission needed | repository **Administration: read and write** (a broad grant: it includes full repository administration) | organization **Self-hosted runners: read and write** |
| `GITHUB_RUNNER_GROUP_ID` | **must be the Default group (ID 1)**: GitHub registers a repository-level runner in the Default group only, and answers 404 for any other group ID (observed on the first real run) | an **organization** runner group |
| Who decides which repositories may use the runner | the controller allowlists only | the controller allowlists **and** the runner group's repository access on GitHub |

Prefer `organization`: it needs a much narrower grant, and the runner group
adds a second, independent limit on which repositories can reach the runners.
With `repository` scope there is no such group-level limit: the runner exists
only on the repository it was created for, and the controller's allowlists are
the only restriction on which repositories get runners.
The controller's own allowlists (`GITHUB_ALLOWED_ORGANIZATIONS`,
`GITHUB_ALLOWED_REPOSITORIES`, labels, forks) apply in both scopes. Webhooks
still identify the repository; the organization is taken from the repository
owner, so a repository owned by a user account cannot use `organization`.

With `organization`, ask for tokens no wider than needed:
`GITHUB_APP_TOKEN_PERMISSIONS=organization_self_hosted_runners=write`.

Status: both scopes are covered by tests against a fake GitHub server. Neither
has run against github.com yet. Whether a free organization can use a custom
runner group, rather than the default group, depends on the GitHub plan:
confirm it in a test organization before relying on it.

## Runners are interchangeable

The controller creates one runner for each queued job, but GitHub does not tie
them together: any idle runner whose labels match takes the next queued job. The
runner created for job A may well run job B (the first live run showed exactly
this). Two consequences are built in:

- **A finished job retires only the runner that ran it.** The `completed` and
  `cancelled` webhooks name that runner (`runner_name`), and the controller
  recognises its own names (`<prefix>-<job number>-<attempt>`). A job that ended
  without ever running (cancelled while queued) names no runner and retires
  none. A runner it did not create is never touched.
- **The reconciler never terminates a runner because its job ended.** A runner
  ends itself after one job; one that never gets a job, or never ends, is removed
  when its lifetime (`RUNNER_MAX_LIFETIME`) expires, by the expiry reaper and the
  provider-side backstop.

The cost of the second rule is that a runner created for a job cancelled while
queued waits, idle, for another job or for its lifetime. Keep the lifetime
short enough that this is acceptable.

## Webhook processing

The endpoint verifies the signature, validates the payload, queues the event,
and answers `202` immediately. Provisioning happens afterwards in the
background. GitHub abandons a delivery after about ten seconds, while starting a
runner takes longer, and a request that GitHub hung up used to cancel the
provision it was waiting for.

| Response | Meaning |
|---|---|
| `202` | accepted; processing continues in the background |
| `401` | missing or wrong signature |
| `400` | unreadable body, body over `WEBHOOK_MAX_BODY_BYTES`, or an unsupported payload |
| `503` | the backlog is full or the controller is shutting down; redeliver the event from GitHub's delivery log |

Guarantees:

- Events for the same job run one at a time, in arrival order, so `completed` can never overtake `queued`.
- Events for different jobs run concurrently, up to `WEBHOOK_WORKERS` (default 16). A slow job never delays another.
- At most `WEBHOOK_QUEUE_SIZE` events (default 1024) wait or run. Beyond that the endpoint answers `503` at once.
- Each event has its own `WEBHOOK_EVENT_TIMEOUT` (default 30 minutes), independent of the request.
- On `SIGTERM` the controller stops accepting events, finishes accepted ones for up to 25 seconds, then cancels what is left. The service's stop timeout (35 seconds in `aws-ecs-controller`) must stay above that.

What it does not do:

- **A failed job is no longer visible in GitHub's delivery log.** GitHub sees `202`. Failures are logged (`workflow job ... failed`) and counted in the `webhook_events_total` metric with `outcome="failed"`. Alert on that counter. A failed event releases its idempotency record, so redelivering it from GitHub retries it.
- **The queue is in memory.** An event acknowledged but not finished when the process is killed is lost. The reconciler terminates any runner that was already created, and the event can be redelivered. A durable queue is a possible follow-up.

Outcomes of `webhook_events_total`: `ok`, `failed`, `abandoned` (dropped at shutdown) and `rejected` (answered `503`).

## Status and known limits

- GitHub App authentication is tested against a fake GitHub server with a
  key generated at test time. It has **not** yet been exercised against
  github.com. Do that in a disposable test repository before trusting it.
- One controller serves one App installation (one organization or account). Serving several
  installations from one controller needs a change to how tokens are requested.
- One controller replica: the coordination package exists but is not wired into
  the executable, so `desired_count` is limited to 0 or 1.
- Webhook processing is asynchronous and in memory; see
  [Webhook processing](#webhook-processing). An accepted event that has not
  finished when the process dies is lost unless it is redelivered.
- `GITHUB_API_BASE_URL` is passed to the client, which allows GitHub Enterprise
  Server, but that has not been tested against a real GHES instance.
- Timeouts: `GITHUB_JIT_REQUEST_TIMEOUT` (default 30s) bounds each GitHub API
  request, `GITHUB_REGISTRATION_TIMEOUT` (default 10m) how long the controller
  waits for a new runner to register, and `GITHUB_REGISTRATION_POLL_INTERVAL`
  (default 5s) how often it asks. A slow image or a loaded machine needs a longer
  registration wait: a runner that has not registered when it expires is
  terminated and its job is failed.
- ALB to task traffic is HTTP inside the VPC, not end-to-end TLS.

Repository-scope JIT also requires repository administration permission; use
organization scope to avoid it (see "Where runners register").
