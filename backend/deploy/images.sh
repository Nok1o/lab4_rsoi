#!/usr/bin/env bash
set -euo pipefail

deploy_directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
backend_directory="$(cd -- "$deploy_directory/.." && pwd)"
mode="${1:-plan}"
target="${2:-all}"
dockerhub_namespace="${DOCKERHUB_NAMESPACE:-nok1o}"
repository="${DOCKERHUB_REPOSITORY:-lab4_rsoi}"
image_tag="${IMAGE_TAG:-preview}"

case "$mode" in
  plan|build|push) ;;
  *)
    echo "Usage: bash backend/deploy/images.sh [plan|build|push] [all|library|rating|reservation|gateway]" >&2
    exit 2
    ;;
esac

case "$target" in
  all) services=(library rating reservation gateway) ;;
  library|rating|reservation|gateway) services=("$target") ;;
  *)
    echo "Unknown service: $target" >&2
    exit 2
    ;;
esac

if [[ "$mode" != plan ]]; then
  : "${IMAGE_TAG:?Set IMAGE_TAG to a new version label, preferably a committed source revision}"
  if [[ "$image_tag" == preview ]]; then
    echo "The preview tag is for plan only. Set a real IMAGE_TAG." >&2
    exit 2
  fi
  docker info >/dev/null
  if [[ "$mode" == build ]]; then
    docker buildx version >/dev/null
  fi
fi

for service in "${services[@]}"; do
  image="docker.io/$dockerhub_namespace/$repository:$service-$image_tag"
  build_arguments=(
    buildx build
    --platform linux/amd64
    --file "$deploy_directory/docker/$service/Dockerfile"
    --tag "$image"
    --load
    "$backend_directory"
  )
  case "$mode" in
    plan)
      printf '%q ' docker "${build_arguments[@]}"
      printf '\n'
      printf '%q ' docker push "$image"
      printf '\n'
      ;;
    build)
      echo "Building $service"
      docker "${build_arguments[@]}"
      ;;
    push)
      echo "Pushing $image"
      docker push "$image"
      ;;
  esac
done
