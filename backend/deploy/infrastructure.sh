#!/usr/bin/env bash
set -euo pipefail

deploy_directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_directory="$(cd -- "$deploy_directory/../.." && pwd)"
chart_directory="$deploy_directory/helm/infrastructure"
namespace="${KUBE_NAMESPACE:-library-system}"
release="${INFRA_RELEASE:-library-infra}"
mode="${1:-template}"

# Helm cannot read files outside its chart. Pass the existing SQL sources as values
# instead of keeping a second copy of the schemas in the chart.
schema_arguments=(
  --set-file "postgresql.schemas.libraries=$project_directory/postgres/scripts/schema-library.sql"
  --set-file "postgresql.schemas.reservations=$project_directory/postgres/scripts/schema-reservation.sql"
  --set-file "postgresql.schemas.ratings=$project_directory/postgres/scripts/schema-rating.sql"
)

case "$mode" in
  lint)
    exec helm lint "$chart_directory" --strict "${schema_arguments[@]}"
    ;;
  template)
    exec helm template "$release" "$chart_directory" \
      --namespace "$namespace" "${schema_arguments[@]}"
    ;;
  deploy)
    context="${KUBE_CONTEXT:-$(kubectl config current-context)}"
    echo "Deploying $release to context=$context namespace=$namespace" >&2
    # Do not generate or overwrite credentials during an upgrade.
    kubectl --context "$context" --namespace "$namespace" get secret \
      postgresql-credentials redis-credentials >/dev/null
    exec helm upgrade --install "$release" "$chart_directory" \
      --kube-context "$context" --namespace "$namespace" \
      "${schema_arguments[@]}" --wait --timeout 10m
    ;;
  *)
    echo "Usage: bash backend/deploy/infrastructure.sh [lint|template|deploy]" >&2
    exit 2
    ;;
esac
