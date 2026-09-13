# Controlled Workload Runner

This offline runner executes a previously validated workload-intake request in
a detached temporary Git worktree. It requires a clean checkout whose `HEAD`
exactly matches the request commit, rejects network-enabled requests, and
accepts only direct, bounded forms of `go test|vet|build ./...`, `cargo test`,
or package-manager `test` commands.

```sh
go run . --request /path/to/intake.json \
  --repository /path/to/clean/checkout \
  --output /tmp/workload-evidence.json
```

The runner never invokes a shell. It applies offline tool settings, limits
runtime and output, redacts secret-shaped output, removes the temporary
worktree, and writes evidence atomically with mode `0600`. A non-zero command
or timeout still produces evidence and exits non-zero; validation or policy
refusals are reported as `BLOCKED`.
