#!/usr/bin/env bash
# publish.sh — build all platforms locally and publish a GitHub Release.
#
# Usage:  ./scripts/publish.sh v0.421
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
DIST="${ROOT_DIR}/dist"

# ── 1. Require a version argument ─────────────────────────────────────────────
if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <version>   e.g. $0 v0.421" >&2
  exit 1
fi

VERSION="${1#v}"          # strip leading 'v' for ldflags  (0.421)
TAG="v${VERSION}"         # always has 'v' prefix           (v0.421)

echo "==> Publishing screener ${TAG}"

# ── 2. Confirm working tree is clean ──────────────────────────────────────────
if [[ -n "$(git -C "${ROOT_DIR}" status --porcelain)" ]]; then
  echo "ERROR: working tree has uncommitted changes. Commit or stash first." >&2
  exit 1
fi

# ── 3. Update VERSION in release.sh so it stays in sync ──────────────────────
sed -i "s/^VERSION=.*/VERSION=\"${VERSION}\"/" "${SCRIPT_DIR}/release.sh"
if [[ -n "$(git -C "${ROOT_DIR}" status --porcelain scripts/release.sh)" ]]; then
  git -C "${ROOT_DIR}" add scripts/release.sh
  git -C "${ROOT_DIR}" commit -m "chore: bump version to ${TAG}"
fi

# ── 4. Build all platforms ────────────────────────────────────────────────────
LOCAL_GO="${HOME}/.local/go1.25.8/go/bin/go"
GO_BIN="$(command -v go)"
[[ -x "${LOCAL_GO}" ]] && GO_BIN="${LOCAL_GO}"

COMMIT=$(git -C "${ROOT_DIR}" rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u +%Y-%m-%d)
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.CommitHash=${COMMIT} -X main.BuildDate=${BUILD_DATE}"

mkdir -p "${DIST}"

declare -A TARGETS=(
  ["linux/amd64"]="screener-linux-amd64"
  ["linux/arm64"]="screener-linux-arm64"
  ["darwin/amd64"]="screener-darwin-amd64"
  ["darwin/arm64"]="screener-darwin-arm64"
  ["windows/amd64"]="screener-windows-amd64.exe"
)

printf '==> vet\n'
"${GO_BIN}" vet "${ROOT_DIR}/..."

printf '==> test\n'
"${GO_BIN}" test "${ROOT_DIR}/..."

for TARGET in "${!TARGETS[@]}"; do
  GOOS="${TARGET%%/*}"
  GOARCH="${TARGET##*/}"
  OUT="${DIST}/${TARGETS[$TARGET]}"
  printf '==> build %s/%s\n' "${GOOS}" "${GOARCH}"
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
    "${GO_BIN}" build -trimpath -ldflags="${LDFLAGS}" -o "${OUT}" "${ROOT_DIR}/cmd/screener"
done

# Copy native binary to dist/screener for convenience
cp "${DIST}/screener-linux-amd64" "${DIST}/screener" 2>/dev/null || true

# ── 5. Zip Windows binary ─────────────────────────────────────────────────────
(cd "${DIST}" && zip -q screener-windows-amd64.zip screener-windows-amd64.exe)

# ── 6. Tag and push ───────────────────────────────────────────────────────────
printf '==> tagging %s\n' "${TAG}"
git -C "${ROOT_DIR}" tag -a "${TAG}" -m "Release ${TAG}"
git -C "${ROOT_DIR}" push origin main
git -C "${ROOT_DIR}" push origin "${TAG}"

# ── 7. Create GitHub Release and upload assets ────────────────────────────────
printf '==> creating GitHub release %s\n' "${TAG}"

ASSETS=(
  "${DIST}/screener-linux-amd64"
  "${DIST}/screener-linux-arm64"
  "${DIST}/screener-darwin-amd64"
  "${DIST}/screener-darwin-arm64"
  "${DIST}/screener-windows-amd64.zip"
)

gh release create "${TAG}" \
  --repo NamasteJasutin/screener \
  --title "screener ${TAG}" \
  --notes "$(cat <<NOTES
## screener ${TAG}

### Downloads

| Platform | File |
|----------|------|
| Linux x86-64 | \`screener-linux-amd64\` |
| Linux ARM64 | \`screener-linux-arm64\` |
| macOS x86-64 | \`screener-darwin-amd64\` |
| macOS Apple Silicon | \`screener-darwin-arm64\` |
| Windows x86-64 | \`screener-windows-amd64.zip\` |

### Linux / macOS

\`\`\`bash
chmod +x screener-linux-amd64
./screener-linux-amd64
\`\`\`

### Windows

Extract \`screener-windows-amd64.zip\` and run \`screener-windows-amd64.exe\` from Windows Terminal or PowerShell.
NOTES
)" \
  "${ASSETS[@]}"

printf '\nDone! Release live at:\n'
printf 'https://github.com/NamasteJasutin/screener/releases/tag/%s\n' "${TAG}"
