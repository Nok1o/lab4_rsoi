#!/usr/bin/env bash
set -euo pipefail

deploy_directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
chart_directory="$deploy_directory/helm/service"
namespace="${KUBE_NAMESPACE:-library-system}"
mode="${1:-template}"
target="${2:-all}"
dockerhub_namespace="${DOCKERHUB_NAMESPACE:-nok1o}"
image_tag="${IMAGE_TAG:-preview}"

case "$mode" in
  lint|template|deploy) ;;
  *)
    echo "Usage: bash backend/deploy/application.sh [lint|template|deploy] [all|library|rating|reservation|gateway]" >&2
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

if [[ "$mode" == deploy ]]; then
  : "${DOCKERHUB_NAMESPACE:?Set DOCKERHUB_NAMESPACE to your Docker Hub user or organization}"
  : "${IMAGE_TAG:?Set IMAGE_TAG to a tag already published for all selected services}"
  if [[ "$image_tag" == preview ]]; then
    echo "The preview tag is for lint/template only. Publish real images before deploying." >&2
    exit 2
  fi
  context="${KUBE_CONTEXT:-$(kubectl config current-context)}"
  echo "Deploying to context=$context namespace=$namespace" >&2
  kubectl --context "$context" --namespace "$namespace" get secret \
    postgresql-credentials redis-credentials >/dev/null
fi

for service in "${services[@]}"; do
  release="$service-service"
  values_file="$deploy_directory/helm/values/$service.yaml"
  arguments=(
    --values "$values_file"
    --set-string "image.namespace=$dockerhub_namespace"
    --set-string "image.tag=$image_tag"
  )
  if [[ -n "${DOCKERHUB_REPOSITORY:-}" ]]; then
    arguments+=(--set-string "image.repository=$DOCKERHUB_REPOSITORY")
  fi

  case "$mode" in
    lint)
      helm lint "$chart_directory" --strict "${arguments[@]}"
      ;;
    template)
      helm template "$release" "$chart_directory" \
        --namespace "$namespace" "${arguments[@]}"
      ;;
    deploy)
      helm upgrade --install "$release" "$chart_directory" \
        --kube-context "$context" --namespace "$namespace" \
        "${arguments[@]}" --wait --timeout 10m
      ;;
  esac
done
