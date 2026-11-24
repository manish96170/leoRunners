# Runner validation workload

A small front-end project that asks a CI runner for what real repositories ask
for. Use it to prove a runner works **before** routing any real workflow to it.
Failures here cost minutes in a repository nobody else uses.

It is generic: copy it into a throwaway repository you own, never into a
production one. It contains no secrets, no deployment, and no cloud account.

## What it exercises

| Runner capability | Where |
|---|---|
| runner registers, picks up the job, exits | every job |
| `actions/checkout`, `setup-node`, `cache`, `upload-artifact`, `download-artifact` | all jobs |
| exact Node version (`versions.env`) | `lint`, checked with `test "$(node --version)" = ...` |
| Yarn install with a restored cache | `lint` (`actions/cache` on the Yarn cache) |
| actionlint, Biome, a custom lint | `lint` |
| unit tests split into shards | `unit` (matrix, `vitest --shard`) |
| a build that writes a **hidden** file (`dist/.vite/manifest.json`) | `build` |
| artifact upload and download keeping hidden files | `build` then `integration` |
| a legacy, **amd64-only** Terraform (0.12.23) installed with a checksum | `integration` |
| a temporary environment: init, apply, health check, destroy, and proof it is gone | `integration` (local state, no cloud) |
| a headless browser test | `integration` (Cypress with Chrome by default, so the image must provide it; `electron` is available but deprecated by Cypress) |
| Docker | `integration`, when `LEO_REQUIRE_DOCKER` is `true` |
| what the runner image provides | `scripts/report-environment.sh` writes versions, architecture, CPUs and free disk to the job summary |

The job summary of the first run is the evidence of what the image contains.

## Where it runs

The workflow reads a repository variable, so the same files run on
GitHub-hosted runners (the fallback) or on LeoRunners without a commit:

| Variable | Meaning |
|---|---|
| `LEO_RUNS_ON` | JSON array of runner labels, for example `["self-hosted","linux","x64","<your approved label>"]`. Unset: `ubuntu-latest`. |
| `LEO_CYPRESS_BROWSER` | `chrome` (default) or `electron` |
| `LEO_REQUIRE_DOCKER` | `true` to require a working Docker daemon |

Run it once on `ubuntu-latest` first. If it fails there, the sample is wrong, not
the runner. Then set `LEO_RUNS_ON` and run it again, and compare the two summaries.

## Safety

- Triggers: pull requests into `main` only, and only from this repository
  (`head.repo.full_name == github.repository`). No push, schedule or
  `workflow_dispatch` trigger.
- Token: `contents: read`. No secrets are referenced. Actions are pinned to full
  commit SHAs.
- Everything it creates lives in `$RUNNER_TEMP` and is destroyed in an
  `if: always()` step; the job fails if the teardown leaves anything behind.
- Do not add credentials to this repository. A temporary cloud resource is a
  separate, later step that needs a role scoped by the runner owner.

## Pinned versions

`versions.env` holds the tool versions and checksums. Terraform 0.12.23 is
chosen because it is old and has no `linux/arm64` build: on an arm64 runner the
`integration` job stops with a clear message, which proves the architecture a
workload needs.

## Local checks

```sh
yarn install
yarn lint && yarn lint:custom && yarn test && yarn build
```

Needs Node 26.8.2 (or change `NODE_VERSION`) and Yarn 1. Cypress and the legacy
Terraform run only in CI.
