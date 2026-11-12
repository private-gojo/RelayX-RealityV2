# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.27.4
ARG XRAY_VERSION=26.11.9

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    mkdir -p /out && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/x-relay ./cmd/x-relay

FROM --platform=$TARGETPLATFORM ghcr.io/xtls/xray-core:${XRAY_VERSION} AS xray

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/x-relay /app/x-relay
COPY --from=xray /usr/local/bin/xray /usr/local/bin/xray

ENV XRAY_BIN=/usr/local/bin/xray
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/x-relay", "server"]
