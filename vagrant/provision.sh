#!/bin/bash
set -euo pipefail

# System update + base tooling.
sudo apt-get update
sudo apt-get upgrade -y
sudo apt-get install -y \
    build-essential \
    ca-certificates \
    curl \
    git \
    libpq-dev \
    postgresql \
    ruby \
    ruby-dev \
    rubygems

# Go toolchain.
GO_VERSION="1.24.6"
GO_ARCHIVE="go${GO_VERSION}.linux-amd64.tar.gz"
if [ ! -d /usr/local/go ] || ! /usr/local/go/bin/go version | grep -q "go${GO_VERSION}"; then
    curl -fsSLO "https://go.dev/dl/${GO_ARCHIVE}"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "${GO_ARCHIVE}"
    rm -f "${GO_ARCHIVE}"
fi

# Make `go` available to the vagrant user on login.
if ! grep -q "/usr/local/go/bin" "$HOME/.profile"; then
    echo 'export PATH=$PATH:/usr/local/go/bin' >> "$HOME/.profile"
fi
export PATH=$PATH:/usr/local/go/bin

# Pre-fetch module dependencies.
if [ -d /vagrant ]; then
    cd /vagrant
    /usr/local/go/bin/go mod download
fi

# Database setup matching the integration test defaults.
sudo systemctl enable --now postgresql
sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname='vagrant'" | grep -q 1 \
    || sudo -u postgres psql -c "CREATE ROLE vagrant WITH LOGIN SUPERUSER PASSWORD 'password';"
sudo -u postgres psql -tc "SELECT 1 FROM pg_database WHERE datname='vagrant'" | grep -q 1 \
    || createdb -U postgres vagrant

# Packaging tools (used by scripts/deb-package.sh).
sudo gem install --no-document fpm
