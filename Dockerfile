# syntax=docker/dockerfile:1

# --- build stage -------------------------------------------------------
FROM golang:1.26.5-bookworm AS build

WORKDIR /src

# Cache module downloads separately from source so `go mod download` only
# reruns when go.mod/go.sum change, not on every source edit.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build args let CI (or `docker build --build-arg`) inject real values;
# internal/buildinfo's own defaults ("dev"/"unknown") cover a plain
# `docker build .` with none supplied. CGO_ENABLED=0 produces a static
# binary: chromedp drives Chrome over the DevTools Protocol (a subprocess +
# websocket), it never links against libchrome, so no cgo is needed here.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w \
    -X github.com/rodvieira/pricing-optimizer-api/internal/buildinfo.Version=${VERSION} \
    -X github.com/rodvieira/pricing-optimizer-api/internal/buildinfo.Commit=${COMMIT} \
    -X github.com/rodvieira/pricing-optimizer-api/internal/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /out/api ./cmd/api

# --- runtime stage -------------------------------------------------------
# debian-slim, not distroless/scratch: the scraper's ChromedpScraper drives
# a real headless Chromium (SPA rendering colly's static fetch can't do),
# which needs a real userland (shared libs, fonts, ca-certificates) that a
# scratch/distroless image deliberately doesn't have.
FROM debian:bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium \
    ca-certificates \
    fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

RUN useradd --system --create-home --shell /usr/sbin/nologin appuser
USER appuser
WORKDIR /home/appuser

COPY --from=build /out/api /usr/local/bin/api

ENV CHROME_EXEC_PATH=/usr/bin/chromium
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/api"]
