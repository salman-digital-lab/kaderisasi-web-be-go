FROM node:24.14.1-trixie-slim AS interop
WORKDIR /interop
COPY tests/interop/package.json tests/interop/package-lock.json ./
RUN npm ci --ignore-scripts --omit=dev

FROM golang:1.26.8-trixie AS build
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal
COPY scripts/crypto-fixture.cjs scripts/vine-fixture.mjs scripts/edge-fixture.mjs ./scripts/
COPY --from=interop /usr/local/bin/node /usr/local/bin/node
COPY --from=interop /interop ./tests/interop
RUN go vet ./... && go test -count=1 -timeout=10m ./... \
    && go build -trimpath -ldflags='-s -w' -o /out/web-api ./cmd/web-api

FROM debian:trixie-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 10001 --create-home app
COPY --from=build /out/web-api /app/web-api
ENV HOST=0.0.0.0 \
    PORT=3333 \
    TZ=Asia/Jakarta \
    NODE_ENV=production
WORKDIR /app
USER app
EXPOSE 3333
HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=3 \
    CMD curl --fail --silent http://127.0.0.1:3333/health || exit 1
CMD ["/app/web-api"]
