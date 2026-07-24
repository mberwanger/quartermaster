#!/usr/bin/env bash
set -euo pipefail

# goreleaser wrapper script
# Ensures consistent build behavior across development and CI environments

GORELEASER_VERSION="v2.13.3"

# Check if goreleaser is installed and matches the expected version
NEEDS_INSTALL=false
if ! command -v goreleaser &>/dev/null; then
    NEEDS_INSTALL=true
elif ! goreleaser --version 2>&1 | grep -q "${GORELEASER_VERSION#v}"; then
    echo "goreleaser version mismatch, upgrading to ${GORELEASER_VERSION}..."
    NEEDS_INSTALL=true
fi

if [ "$NEEDS_INSTALL" = true ]; then
    echo "Installing goreleaser ${GORELEASER_VERSION}..."
    go install github.com/goreleaser/goreleaser/v2@${GORELEASER_VERSION}
fi

# Run goreleaser with the provided arguments
exec goreleaser "$@"