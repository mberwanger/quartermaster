#!/bin/bash
set -euo pipefail

REPO_ROOT="${REPO_ROOT:-"$(realpath "$(dirname "${BASH_SOURCE[0]}")/..")"}"
BUILD_ROOT="${REPO_ROOT}/build"
BUILD_BIN="${BUILD_ROOT}/bin"

NAME=golangci-lint
# The linter must be built with a Go at least as new as the toolchain that
# compiles this module, or it cannot load the packages at all. Bump this when
# bumping Go.
RELEASE=v2.12.2
DARWIN_AMD64_256=f6f06d94b6241521c53d15450c5209b028270bf966f842afb11c030c79f5bc16
DARWIN_ARM64_256=a9c54498731b3128f79e090be6110f3e5fffccc617b08142ed244d4126c73f29
LINUX_AMD64_256=8df580d2670fed8fa984aac0507099af8df275e665215f5c7a2ae3943893a553
LINUX_ARM64_256=44cd40a8c76c86755375adfeea52cfd3533cb43d7bd647771e0ae065e166df3a

case "$(uname -m)" in
  arm64 | aarch64) ARCH=arm64 ;;
  *) ARCH=amd64 ;;
esac

RELEASE_BINARY="${BUILD_BIN}/${NAME}-${RELEASE}"

main() {
  ensure_binary

  "${RELEASE_BINARY}" "$@"
}

ensure_binary() {
  if [[ ! -f "${RELEASE_BINARY}" ]]; then
    echo "info: Downloading ${NAME} ${RELEASE} to build environment"

    mkdir -p "${BUILD_BIN}"

    case "${OSTYPE}-${ARCH}" in
    darwin*-amd64) os_type="darwin"; sum="${DARWIN_AMD64_256}" ;;
    darwin*-arm64) os_type="darwin"; sum="${DARWIN_ARM64_256}" ;;
    linux*-amd64) os_type="linux"; sum="${LINUX_AMD64_256}" ;;
    linux*-arm64) os_type="linux"; sum="${LINUX_ARM64_256}" ;;
    *) echo "error: Unsupported platform '${OSTYPE}/${ARCH}' for ${NAME} install, please install manually" && exit 1 ;;
    esac

    release_archive="/tmp/${NAME}-${RELEASE}.tar.gz"

    URL="https://github.com/golangci/golangci-lint/releases/download/${RELEASE}/golangci-lint-${RELEASE:1}-${os_type}-${ARCH}.tar.gz"
    curl -sSL -o "${release_archive}" "${URL}"
    echo "${sum}" "${release_archive}" | sha256sum --check --quiet -

    release_tmp_dir="/tmp/${NAME}-${RELEASE}"
    mkdir -p "${release_tmp_dir}"
    tar -xzf "${release_archive}" --strip=1 -C "${release_tmp_dir}"

    if [[ ! -f "${RELEASE_BINARY}" ]]; then
      find "${BUILD_BIN}" -maxdepth 0 -regex '.*/'${NAME}'-[A-Za-z0-9\.]+$' -exec rm {} \; # cleanup older versions
      mv "${release_tmp_dir}/${NAME}" "${RELEASE_BINARY}"
    fi

    # Cleanup stale resources.
    rm "${release_archive}"
    rm -rf "${release_tmp_dir}"
  fi
}

main "$@"
