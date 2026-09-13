# Runner Controller

Phase 1 is a Go controller foundation for ephemeral GitHub Actions runners. It provides workflow event normalization, HMAC webhook verification, optimistic in-memory state, a provider-neutral scheduler, a configurable fake provider, lifecycle cleanup, and a local HTTP entrypoint.

## Run locally

```bash
GITHUB_WEBHOOK_SECRET=development go run ./cmd/runner-controller
```

The controller listens on `:8080`. The Phase 1 executable uses an in-memory repository and fake provider; it does not provision AWS or register a real GitHub runner yet.

## Verify

```bash
gofmt -w cmd internal
go test ./...
go test -race ./...
go vet ./...
```
