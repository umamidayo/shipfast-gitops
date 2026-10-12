#!/usr/bin/env bash
# Build the status-check image, push it to the local Kind registry, and pin the
# new tag in deploy/overlays/local. Argo CD deploys it once the commit is pushed.
#
#   scripts/release.sh             build, push image, commit, git push
#   scripts/release.sh --no-push   same, but leave the commit for you to push
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
REGISTRY="${REGISTRY:-localhost:5001}"
OVERLAY="deploy/overlays/local/kustomization.yaml"
GIT_PUSH=1
[[ "${1:-}" == "--no-push" ]] && GIT_PUSH=0

cd "$REPO_ROOT"

# The image tag is the commit that produced it, so uncommitted app changes
# would make the tag lie about its contents.
if [[ -n "$(git status --porcelain -- app)" ]]; then
  echo "error: app/ has uncommitted changes; commit them first so the image tag matches its source." >&2
  exit 1
fi

TAG="$(git rev-parse --short HEAD)"
IMAGE="$REGISTRY/status-check:$TAG"

echo "==> Building $IMAGE"
docker build --build-arg VERSION="$TAG" -t "$IMAGE" app

echo "==> Pushing $IMAGE"
docker push "$IMAGE"

echo "==> Pinning $TAG in $OVERLAY"
sed -i.bak -E "s|^([[:space:]]*newTag:).*|\1 \"$TAG\"|" "$OVERLAY" && rm -f "$OVERLAY.bak"
git add "$OVERLAY"
if git diff --cached --quiet; then
  echo "Overlay already at $TAG; nothing to commit."
  exit 0
fi
git commit -m "deploy(local): status-check $TAG"

if [[ $GIT_PUSH == 1 ]]; then
  echo "==> Pushing to git; Argo CD will sync on its next poll"
  git push
else
  echo "Committed. Run 'git push' to deploy."
fi
