# syntax=docker/dockerfile:1

ARG BUILDPLATFORM
ARG TARGETOS=linux
ARG TARGETARCH

# Build natively, then cross-compile the small static controller binary for
# the runner target. This avoids executing the Go toolchain through emulation.
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS build

ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /src/controller

# Keep dependency downloads in a separate layer when source files change.
COPY controller/go.mod controller/go.sum ./
RUN go mod download

COPY controller/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/runner-controller ./cmd/runner-controller

FROM alpine:3.22@sha256:2e0b81d60597f449baa56dc25d355367869d2dca4cd57e866f1e2b52af3eb55f AS runtime

RUN addgroup -S -g 65532 runner \
    && adduser -S -D -H -u 65532 -G runner runner \
    && mkdir -p /var/lib/leo-runners \
    && chown runner:runner /var/lib/leo-runners

COPY --from=build --chown=runner:runner /out/runner-controller /usr/local/bin/runner-controller

ENV LISTEN_ADDR=:8080 \
    STATE_PATH=/var/lib/leo-runners/state.json \
    AI_ENABLED=false

EXPOSE 8080
VOLUME ["/var/lib/leo-runners"]
USER 65532:65532
STOPSIGNAL SIGTERM

ENTRYPOINT ["/usr/local/bin/runner-controller"]
