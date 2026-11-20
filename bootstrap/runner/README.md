# Secure ephemeral runner bootstrap

`bootstrap.sh` starts a GitHub Actions runner in JIT mode. It is designed to
run from a pre-baked runner image, with the short-lived JIT configuration
injected at launch time by the controller through a private channel.

## Contract

Required environment:

```text
GITHUB_JIT_CONFIG_B64  GitHub's encoded_jit_config value, passed unchanged
```

The controller must inject `GITHUB_JIT_CONFIG_B64` through the instance's
private bootstrap channel (for example, one-time user-data or supervisor
environment injection) and must never put the value in an image, repository,
AMI metadata, tags, logs, or a long-lived file. The channel must be protected
from untrusted local users and must not be exposed through public metadata.

Alternative one-time input channels (use exactly one input source):

```text
GITHUB_JIT_CONFIG_FILE  absolute path to a private file containing encoded_jit_config
GITHUB_JIT_CONFIG_FD    numeric open file descriptor containing encoded_jit_config
```

The file or descriptor is copied once into a mode-0600 private staging file;
the file descriptor is closed immediately afterward. These channels avoid
placing the secret in the process environment. The runner itself requires the
encoded value as the `--jitconfig` argument, so the short-lived value may still
be visible in that child process's argument list while registration runs.

Optional environment:

```text
RUNNER_DIR         runner installation directory; defaults to this directory
RUNNER_RUN_SCRIPT  runner entrypoint, absolute or relative to RUNNER_DIR; defaults to RUNNER_DIR/run.sh
JIT_CONFIG_FILE    absolute private staging-file path; otherwise a private temp file is used
```

The default runner entrypoint is invoked as:

```text
run.sh --jitconfig <encoded-jit-config>
```

GitHub JIT configuration is the ephemeral one-job registration path. The
bootstrap does not call `config.sh`, create a persistent registration, or add
an ordinary non-ephemeral runner. The runner exits after its assigned job and
the controller should terminate the instance afterward.

## User-data template (`user-data.sh`)

`user-data.sh` is the launch-time script for instances whose image does not
already contain the GitHub runner. Point `RUNNER_USER_DATA_FILE` at it. The
controller replaces `{{GITHUB_JIT_CONFIG_B64}}` (exactly one occurrence) with the
one-time JIT value, shell-quoted, and base64-encodes the result for EC2.

It, in order:

1. uses a runner already installed in `/opt/actions-runner` if the image has one;
   otherwise downloads the **pinned** runner release for the machine's
   architecture and **verifies its SHA-256 before extracting anything**, then
   installs the runtime dependencies;
2. creates an unprivileged `runner` user (the runner refuses to run as root);
3. hands the JIT value to that user through a private file that is deleted as
   soon as it is read;
4. runs exactly one job, then powers the instance off.

The pinned version and hashes are at the top of the file. GitHub stops accepting
registrations from runners that are too old, so re-pin regularly:

```sh
bootstrap/runner/pin-latest-runner.sh   # prints the values; review, then edit user-data.sh
```

Downloading at every launch is slow (about a minute on a fast connection) and
depends on GitHub being reachable. Production images should pre-bake the runner (`ami/`); this template
is the fallback and the path used by `tools/local-rehearsal`.

Things to know:

- Anything on the instance, including workflow code, can read user data from the
  metadata service. The JIT value is single use and is spent as soon as the runner
  registers, so a later reader gets nothing usable. IMDSv2 with a hop limit of 1
  (enforced by the AWS provider) keeps containers on the instance from reaching it.
- The template never enables shell tracing, which would print the JIT value into
  the instance logs. A test enforces this.
- Tests: `./test.sh` (static checks) and the controller's
  `TestShippedUserData*` tests, which render the real file through the provider.

## Security and lifecycle behavior

- `umask 077` and `chmod 600` protect the encoded temporary file.
- The encoded value is validated only for presence and non-empty content. It is
  opaque to this script and is never base64-decoded; GitHub's runner expects
  the exact `encoded_jit_config` string returned by its API.
- Temporary sensitive files are removed on normal exit, failure, and signals.
- `TERM`, `INT`, and `HUP` are forwarded to the runner process before cleanup.
- A missing executable, missing payload, empty payload, or runner failure
  produces a non-zero exit status.
- The script does not contain a token, PAT, private key, or other secret.

The process environment and command-line arguments can be observed by a
privileged host user. The deployment channel must therefore be limited to the
runner instance and the instance should be treated as disposable.

## Validation

Run the shell-level validation from this directory:

```sh
./test.sh
```

The test uses a temporary fake `run.sh`; it does not contact GitHub or start a
cloud instance.
