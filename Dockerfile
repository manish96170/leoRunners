# syntax=docker/dockerfile:1

FROM golang:1.27.1-alpine AS build

WORKDIR /src/controller

# Keep dependency downloads in a separate layer when source files change.
COPY controller/go.mod controller/go.sum ./
RUN go mod download

COPY controller/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/runner-controller ./cmd/runner-controller

FROM alpine:3.22 AS runtime

RUN addgroup -S -g 65532 runner \
    && adduser -S -D -H -u 65532 -G runner runner \
    && mkdir -p /var/lib/leo-runners \
    && chown runner:runner /var/lib/leo-runners

COPY --from=build --chown=runner:runner /out/runner-controller /usr/local/bin/runner-controller

ENV LISTEN_ADDR=:8080 \
    STATE_PATH=/var/lib/leo-runners/state.json

EXPOSE 8080
VOLUME ["/var/lib/leo-runners"]
USER 65532:65532
STOPSIGNAL SIGTERM

ENTRYPOINT ["/usr/local/bin/runner-controller"]
