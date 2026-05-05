# syntax=docker/dockerfile:1

# Stage 1: build the gollard binary.
FROM golang:1.24 AS build

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Build.
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/gollard ./cmd/gollard


# Stage 2: package as a Debian package via fpm.
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive \
    DEBFULLNAME="gollard maintainers" \
    DEBEMAIL="gollard@example.com"

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ruby \
        ruby-dev \
        rubygems \
        build-essential \
    && rm -rf /var/lib/apt/lists/*

RUN gem install --no-document fpm

WORKDIR /work

COPY --from=build /out/gollard /tmp/installdir/usr/local/bin/gollard

RUN fpm -s dir -t deb -n gollard -v 0.6.3.0 -C /tmp/installdir \
        --description "gollard - Database management for pedantic people." \
        --license MIT \
        --architecture amd64 \
        usr/local/bin/gollard

CMD cp gollard_0.6.3.0_amd64.deb /dist/
