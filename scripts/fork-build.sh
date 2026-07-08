#!/usr/bin/env bash
#
# fork-build.sh — Build better-cpa Docker image for the fork
#
# Usage:
#   ./scripts/fork-build.sh              # Build Go binary + Docker image
#   ./scripts/fork-build.sh --image-only # Skip Go build, build Docker only
#   ./scripts/fork-build.sh --push       # Build + push to registry
#   ./scripts/fork-build.sh --help       # Show help
#
# Tags:
#   better-cpa:{version}   — e.g. better-cpa:v1.0.0 (from git describe)
#   better-cpa:latest      — always updated on successful build
#

set -euo pipefail

# --- Config ---
REGISTRY="${BETTER_CPA_REGISTRY:-ghcr.io/oleksii-honchar}"
IMAGE_NAME="better-cpa"
PUSH="${BETTER_CPA_PUSH:-false}"

# --- Parse args ---
DO_BUILD_GO=true
DO_PUSH=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image-only) DO_BUILD_GO=false; shift ;;
    --push)       DO_PUSH=true; shift ;;
    --help|-h)    sed -n '/^#/p; /^$/q' "$0" | sed 's/^# //;s/^#$//'; exit 0 ;;
    *)            echo "Error: unknown option '$1'"; exit 1 ;;
  esac
done

# --- Version info ---
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo "dev")"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "none")"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "═══ better-cpa fork build ═══"
echo "  Version: ${VERSION}"
echo "  Commit:  ${COMMIT}"
echo "  Date:    ${BUILD_DATE}"
echo "─────────────────────────────"

# --- Step 1: Verify Go build ---
if $DO_BUILD_GO; then
  echo "→ Building Go binary..."
  go build -buildvcs=false \
    -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" \
    -o /dev/null ./cmd/server/
  echo "  ✓ Go build successful"
fi

# --- Step 2: Run tests ---
echo "→ Running tests..."
go test ./internal/runtime/executor/... -count=1 2>&1 | tail -3
echo "  ✓ Tests complete"

# --- Step 3: Build Docker image ---
echo "→ Building Docker image..."
docker build \
  --build-arg VERSION="${VERSION}" \
  --build-arg COMMIT="${COMMIT}" \
  --build-arg BUILD_DATE="${BUILD_DATE}" \
  -t "${IMAGE_NAME}:${VERSION}" \
  -t "${IMAGE_NAME}:latest" \
  .

echo "  ✓ Docker image built: ${IMAGE_NAME}:${VERSION}"

# --- Step 4: Push (optional) ---
if $DO_PUSH; then
  REMOTE_TAG="${REGISTRY}/${IMAGE_NAME}:${VERSION}"
  REMOTE_LATEST="${REGISTRY}/${IMAGE_NAME}:latest"
  echo "→ Tagging for registry..."
  docker tag "${IMAGE_NAME}:${VERSION}" "${REMOTE_TAG}"
  docker tag "${IMAGE_NAME}:latest" "${REMOTE_LATEST}"
  echo "→ Pushing..."
  docker push "${REMOTE_TAG}"
  docker push "${REMOTE_LATEST}"
  echo "  ✓ Pushed: ${REMOTE_TAG}"
fi

echo "═══ Build complete ═══"
