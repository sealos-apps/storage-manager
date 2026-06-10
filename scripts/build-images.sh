#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

REGISTRY="${REGISTRY:-}"
IMAGE_PREFIX="${IMAGE_PREFIX:-storage-manager}"
TAGS="${TAGS:-${TAG:-dev}}"
PLATFORMS="${PLATFORMS:-linux/amd64}"
PUSH="${PUSH:-false}"
ENCORE="${ENCORE:-encore}"
DOCKER="${DOCKER:-docker}"

if [ -n "$REGISTRY" ]; then
	IMAGE_BASE="${REGISTRY%/}/${IMAGE_PREFIX}"
else
	IMAGE_BASE="$IMAGE_PREFIX"
fi

BACKEND_IMAGE="${BACKEND_IMAGE:-${IMAGE_BASE}-backend}"
WEB_IMAGE="${WEB_IMAGE:-${IMAGE_BASE}-web}"

set -- $TAGS
if [ "$#" -eq 0 ]; then
	echo "No image tags provided through TAG or TAGS." >&2
	exit 1
fi

PRIMARY_TAG="$1"

cd "$ROOT_DIR"

echo "Building backend image ${BACKEND_IMAGE}:${PRIMARY_TAG}"
"$ENCORE" build docker --config=infra-config.json "${BACKEND_IMAGE}:${PRIMARY_TAG}"

shift
for tag in "$@"; do
	echo "Tagging backend image ${BACKEND_IMAGE}:${tag}"
	"$DOCKER" tag "${BACKEND_IMAGE}:${PRIMARY_TAG}" "${BACKEND_IMAGE}:${tag}"
done

echo "Building web image ${WEB_IMAGE}:${PRIMARY_TAG}"
WEB_OUTPUT_ARGS=""
if [ "$PUSH" = "true" ]; then
	WEB_OUTPUT_ARGS="--push"
else
	WEB_OUTPUT_ARGS="--load"
fi

# shellcheck disable=SC2086
"$DOCKER" buildx build \
	--platform "$PLATFORMS" \
	-f web/Dockerfile \
	$(for tag in $TAGS; do printf '%s ' "-t ${WEB_IMAGE}:${tag}"; done) \
	$WEB_OUTPUT_ARGS \
	web

if [ "$PUSH" = "true" ]; then
	for tag in $TAGS; do
		echo "Pushing backend image ${BACKEND_IMAGE}:${tag}"
		"$DOCKER" push "${BACKEND_IMAGE}:${tag}"
	done
fi
