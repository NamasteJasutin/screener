#!/usr/bin/env bash
# release.sh — build, test, and package screener binaries for all supported platforms.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

LOCAL_GO="${HOME}/.local/go1.25.8/go/bin/go"
if [[ -x "${LOCAL_GO}" ]]; then
  GO_BIN="${LOCAL_GO}"
else
  GO_BIN="$(command -v go)"
fi

DIST="${ROOT_DIR}/dist"
mkdir -p "${DIST}"

# ── 1. Validate ───────────────────────────────────────────────────────────────
printf '==> vet\n'
"${GO_BIN}" vet ./...

printf '==> test (with race detector)\n'
"${GO_BIN}" test -race ./...

# ── 2. Build matrix ───────────────────────────────────────────────────────────
VERSION="0.430"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u +%Y-%m-%d)
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.CommitHash=${COMMIT} -X main.BuildDate=${BUILD_DATE}"

printf '==> version %s (%s, %s)\n' "${VERSION}" "${COMMIT}" "${BUILD_DATE}"

declare -A TARGETS=(
  ["linux/amd64"]="screener-linux-amd64"
  ["linux/arm64"]="screener-linux-arm64"
  ["darwin/amd64"]="screener-darwin-amd64"
  ["darwin/arm64"]="screener-darwin-arm64"
  ["windows/amd64"]="screener-windows-amd64.exe"
)

for TARGET in "${!TARGETS[@]}"; do
  GOOS="${TARGET%%/*}"
  GOARCH="${TARGET##*/}"
  OUT="${DIST}/${TARGETS[$TARGET]}"

  printf '==> build %s/%s → %s\n' "${GOOS}" "${GOARCH}" "${TARGETS[$TARGET]}"
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
    "${GO_BIN}" build \
      -trimpath \
      -ldflags="${LDFLAGS}" \
      -o "${OUT}" \
      ./cmd/screener
done

# ── 3. Convenience symlinks for the native platform ───────────────────────────
NATIVE_GOOS="$(${GO_BIN} env GOOS)"
NATIVE_GOARCH="$(${GO_BIN} env GOARCH)"
NATIVE_KEY="${NATIVE_GOOS}/${NATIVE_GOARCH}"
if [[ -n "${TARGETS[$NATIVE_KEY]:-}" ]]; then
  cp "${DIST}/${TARGETS[$NATIVE_KEY]}" "${DIST}/screener"
  printf '==> copied native binary → dist/screener\n'
fi

# ── 4. Checksums ─────────────────────────────────────────────────────────────
printf '==> checksums\n'
(cd "${DIST}" && sha256sum screener-* > SHA256SUMS)
# Also write individual .sha256 files for per-artifact verification.
for TARGET in "${!TARGETS[@]}"; do
  BIN="${DIST}/${TARGETS[$TARGET]}"
  if [[ -f "${BIN}" ]]; then
    sha256sum "${BIN}" > "${BIN}.sha256"
  fi
done
if [[ -f "${DIST}/screener" ]]; then
  sha256sum "${DIST}/screener" > "${DIST}/screener.sha256"
fi

printf '\nRelease artifacts:\n'
ls -lh "${DIST}"/screener* 2>/dev/null || true
printf '\nSHA256SUMS:\n'
cat "${DIST}/SHA256SUMS"
