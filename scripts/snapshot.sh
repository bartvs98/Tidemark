#!/bin/sh
# Export a Docker host's container + image metadata for Tidemark development.
#
#   sh snapshot.sh [output-file]      (default: tidemark-snapshot.json)
#
# Kept:    container id/name/image/state/created, compose project/service labels,
#          tidemark.* and org.opencontainers.image.* labels; image id/tags/digests/
#          created/platform and image labels (the same data any registry serves).
# Dropped: environment variables, commands, mounts, ports, networks, every other label.
#
# Only reads (GET) from the Docker socket. Needs curl and access to the socket
# (root or the docker group). jq runs in a throwaway container, so it needn't be installed.
set -eu

SOCK=${DOCKER_SOCK:-/var/run/docker.sock}
OUT=${1:-tidemark-snapshot.json}
JQ_IMAGE=ghcr.io/jqlang/jq:1.7.1

api() { curl -sSf --unix-socket "$SOCK" "http://localhost$1"; }
jq() { docker run --rm -i "$JQ_IMAGE" "$@"; }

echo "Reading Docker API via $SOCK ..." >&2

version=$(api /version | jq -c '{Version, ApiVersion, Os, Arch}')

containers=$(api '/containers/json?all=1' | jq -c '
  [ .[] | {
      Id, Names, Image, ImageID, Created, State, Status,
      Labels: ((.Labels // {}) | with_entries(select(.key | test(
        "^(com\\.docker\\.compose\\.(project|service)$|tidemark\\.|org\\.opencontainers\\.image\\.)"
      ))))
  } ]')

ids=$(printf '%s' "$containers" | jq -r '.[].ImageID' | tr -d '\r' | sort -u)

images=$(for id in $ids; do api "/images/$id/json" || true; echo; done | jq -s -c '
  [ .[] | {
      Id, RepoTags, RepoDigests, Created, Architecture, Os, Variant,
      Config: { Labels: (.Config.Labels // {}) }
  } ]')

printf '{"capturedAt":"%s","version":%s,"containers":%s,"images":%s}\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$version" "$containers" "$images" | jq . > "$OUT"

echo "Wrote $OUT: $(printf '%s' "$containers" | jq length) containers, $(printf '%s' "$images" | jq length) images." >&2
echo "Review it before sharing. It contains no environment variables." >&2
