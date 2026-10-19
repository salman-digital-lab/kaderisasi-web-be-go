FROM node:24.14.1-trixie-slim AS interop
WORKDIR /interop
COPY tests/interop/package.json tests/interop/package-lock.json ./
RUN npm ci --ignore-scripts --omit=dev

FROM golang:1.26.8-trixie AS build

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential curl ca-certificates xz-utils meson ninja-build pkg-config libglib2.0-dev libexpat1-dev \
    libjpeg62-turbo-dev libpng-dev libwebp-dev libexif-dev liblcms2-dev \
    liborc-0.4-dev zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fL --retry 3 --max-time 600 \
    https://github.com/libvips/libvips/releases/download/v8.18.6/vips-8.18.6.tar.xz \
    -o /tmp/vips.tar.xz \
    && echo '3c41e1d5458081bfa4a5bc54e116c46259c75c6760a18027764555632b9dda3e  /tmp/vips.tar.xz' | sha256sum -c - \
    && tar -xJf /tmp/vips.tar.xz -C /tmp \
    && meson setup /tmp/vips-build /tmp/vips-8.18.6 \
       --prefix=/opt/vips --libdir=lib --buildtype=release --auto-features=disabled \
       -Dintrospection=disabled -Dexamples=false -Dcplusplus=false \
       -Dmodules=disabled -Djpeg=enabled -Dpng=enabled -Dwebp=enabled \
       -Dexif=enabled -Dlcms=enabled -Dorc=enabled -Dzlib=enabled \
    && meson compile -C /tmp/vips-build -j 2 \
    && meson install -C /tmp/vips-build \
    && rm -rf /tmp/vips*

ENV CGO_ENABLED=1 \
    GOTOOLCHAIN=local \
    PKG_CONFIG_PATH=/opt/vips/lib/pkgconfig \
    LD_LIBRARY_PATH=/opt/vips/lib \
    GOMAXPROCS=2
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal
COPY scripts/crypto-fixture.cjs scripts/vine-fixture.mjs scripts/edge-fixture.mjs scripts/sanitize-fixture.mjs \
    scripts/filetype-fixture.mjs scripts/collate-fixture.mjs scripts/pdf-fixture.mjs ./scripts/
COPY --from=interop /usr/local/bin/node /usr/local/bin/node
COPY --from=interop /interop ./tests/interop
RUN go vet ./... && go test -count=1 -timeout=10m ./... \
    && go build -trimpath -ldflags='-s -w' -o /out/web-api ./cmd/web-api

FROM debian:trixie-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl tzdata libglib2.0-0t64 libexpat1 libjpeg62-turbo \
    libpng16-16t64 libwebp7 libwebpdemux2 libwebpmux3 libexif12 liblcms2-2 \
    liborc-0.4-0 zlib1g \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 10001 --create-home app
COPY --from=build /opt/vips /opt/vips
COPY --from=build /out/web-api /app/web-api
ENV LD_LIBRARY_PATH=/opt/vips/lib \
    HOST=0.0.0.0 \
    PORT=3333 \
    TZ=Asia/Jakarta \
    NODE_ENV=production
WORKDIR /app
USER app
EXPOSE 3333
HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=3 \
    CMD curl --fail --silent http://127.0.0.1:3333/health || exit 1
CMD ["/app/web-api"]
