# Minimal runtime image for the gollard CLI used as a one-shot
# migration applier in docker-compose.
#
# Multi-stage: full Go toolchain in `build`, distroless in the final
# image. Result is ~10MB and contains nothing but the binary.

FROM golang:1.24-alpine AS build
ENV CGO_ENABLED=0 GOOS=linux
WORKDIR /src

# Cache the module graph before copying the rest of the source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/gollard ./cmd/gollard

# `static-debian12` ships only a shell-less runtime + ca-certs. It's
# 2MB and has nothing for an attacker to pivot through if the binary
# is ever exposed beyond the compose network.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gollard /usr/local/bin/gollard
ENTRYPOINT ["/usr/local/bin/gollard"]
