# Local rehearsal

Runs the whole LeoRunners flow on one machine, with nothing in the cloud:

```text
GitHub ──webhook──> Cloudflare quick tunnel ──> controller (this machine)
                                                    │  AWS SDK, pointed at floci
                                                    ▼
                                  floci (local AWS emulator, Docker)
                                                    │  RunInstances = a container
                                                    ▼
                         runner container: downloads the GitHub runner, registers
                         with a one-time JIT config, runs ONE job, exits
```

Use it to learn the system, to try a repository's pull-request workflow on your
own runner, and to check a change before it goes near a real AWS account.

It is a rehearsal, not a performance or security test: floci containers are not
EC2 instances, security groups and IAM are not enforced, and start-up times say
nothing about real EC2.

## What it needs

| Tool | Why |
|---|---|
| Docker (for example Colima) | floci and the runner containers |
| `aws`, `terraform`, `go`, `curl`, `openssl`, `python3` | setup and the controller build |
| `cloudflared` | free quick tunnel, only for the real-GitHub stage |
| the floci image (`floci/floci`) | the AWS emulator, pulled on first `floci.sh up` |

Everything it creates is under `~/.leo-runners-rehearsal` (override with
`LEO_REHEARSAL_HOME`). Every AWS call goes to `http://127.0.0.1:4566`: the
scripts hide all AWS profiles and credentials and refuse to continue unless the
endpoint answers as the emulator (account `000000000000`).

The runner containers run whatever the repository's workflow says, on your
machine, with network access. Only point this at repositories you trust.

## Stage 1: webhook intake, no GitHub

```sh
tools/local-rehearsal/floci.sh up
tools/local-rehearsal/setup.sh
tools/local-rehearsal/run-controller.sh           # JIT off: no runner registers
tools/local-rehearsal/send-webhook.sh queued 101  # expect HTTP 202 immediately
tools/local-rehearsal/status.sh                   # an instance appears in floci
tools/local-rehearsal/send-webhook.sh completed 101
tools/local-rehearsal/status.sh                   # ...and is terminated
tools/local-rehearsal/down.sh
```

The `202` returns at once even on a cold start; provisioning continues in the
background.

## Stage 2: a real repository

You need a **test repository you control**, preferably in a test organization.
Runner groups and organization-level settings only exist for organizations, and
a free organization is enough to start.

### 1. Create the GitHub App (in the browser)

Settings, Developer settings, GitHub Apps, **New GitHub App**:

| Setting | Value |
|---|---|
| Name | anything unique, for example `my-leorunners-test` |
| Homepage URL | any URL (your repository's, for example) |
| Webhook | **Active**. URL: set it after step 3 (`tunnel.sh` prints it). Secret: the value of `LEO_WEBHOOK_SECRET`, default `rehearsal-webhook-secret`. Change it for anything beyond a throwaway test |
| Repository permissions | **Actions: Read-only**, plus **Administration: Read and write** for the default repository scope. Nothing else. (Metadata read is added automatically) |
| Organization permissions | Only for `GITHUB_RUNNER_SCOPE=organization` (needs an organization): **Self-hosted runners: Read and write**. Then the App does not need Administration. |
| Subscribe to events | **Workflow job** |
| Where can this App be installed? | Only on this account |

Create it, then on the App's page:

1. Note the **App ID** (or Client ID).
2. **Generate a private key**. A `.pem` file downloads.
3. **Install App**, and choose **only the test repository**.

### 2. Put the key in floci and find the installation

```sh
tools/local-rehearsal/store-app-key.sh ~/Downloads/<your-app>.private-key.pem
rm ~/Downloads/<your-app>.private-key.pem          # the copy in floci is enough
tools/local-rehearsal/app-installations.sh <app-id>   # prints the installation id
```

### 3. Configure and start

```sh
cp tools/local-rehearsal/github.env.example ~/.leo-runners-rehearsal/github.env
$EDITOR ~/.leo-runners-rehearsal/github.env     # app id, installation id, runner group id, allowed repo
tools/local-rehearsal/run-controller.sh --jit
tools/local-rehearsal/tunnel.sh                 # prints the webhook URL
```

Paste the printed URL into the App's webhook URL (App settings, General).
The URL changes each time the tunnel restarts.

The controller logs `GitHub App authentication ok` shortly after starting. If it
logs a check failure instead, the App ID, installation ID or key is wrong.

### 4. Run a job

In the test repository, a workflow whose job uses the labels the controller was
given (default `self-hosted, linux, <your architecture>`):

```yaml
name: rehearsal
on: pull_request
permissions: {}
jobs:
  hello:
    runs-on: [self-hosted, linux, arm64]   # x64 on an Intel host
    steps:
      - uses: actions/checkout@v4
      - run: uname -a && echo "hello from a LeoRunners runner"
```

Open a pull request. Then watch:

```sh
tools/local-rehearsal/status.sh
tail -f ~/.leo-runners-rehearsal/logs/controller.log
```

Expect: the delivery shows `202` in the App's "Advanced" delivery log, a runner
container appears, registers, runs the job, and is removed when GitHub reports
the job completed.

Labels must match exactly. A job asking for a label the controller does not allow
is rejected, and forks are denied by default.

## Check that expired runners are cleaned up

```sh
tools/local-rehearsal/check-reaper.sh
```

About two minutes, no GitHub needed. It checks that:

1. a runner whose expiry passes while the controller is down is terminated after
   the controller restarts, and its record survives;
2. instances the controller has **no record of** are removed only when they are
   ours and past their own expiry. Instances that are still valid, have no
   expiry marker, or are not tagged as ours are left alone.

It exists because the first run of this check found that a runner could be
orphaned: the table's TTL fired at the same instant the reaper needed the
record. See `docs/aws-provider.md`.

## Tear down

```sh
tools/local-rehearsal/down.sh           # stop controller and tunnel
tools/local-rehearsal/down.sh --purge   # also remove floci and ~/.leo-runners-rehearsal
```

Delete the GitHub App (or uninstall it) when you are done. The private key lives
only in floci; `--purge` removes it.

## Known rehearsal limits

- **A runner takes about a minute and a half to start** (measured on Apple
  Silicon): floci's own setup, then downloading and verifying the runner
  (about 140 MB on Arm) and installing its dependencies. The user-data fallback is
  the slow path; a real deployment pre-bakes the runner into the image.
- **Floci runs a runner's user-data only after its own setup**, roughly 30 to 60
  seconds after launch, so do not terminate an instance in that window.
- Docker-in-Docker, service containers and workflows that need a real EC2
  environment will not behave like production.
- The architecture must match the Docker host (Arm on Apple Silicon); emulated
  x86 is very slow.
