#!/usr/bin/env bash
set -euo pipefail

deploy_directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mode="${1:-template}"
chart_version=0.15.1
chart_registry=oci://ghcr.io/actions/actions-runner-controller-charts
case "$mode" in
  template|deploy) ;;
  *) echo "Usage: bash backend/deploy/runner.sh [template|deploy]" >&2; exit 2 ;;
esac

if [[ "$mode" == deploy ]]; then
  context="${KUBE_CONTEXT:-$(kubectl config current-context)}"
  # Both namespaces and the GitHub credential Secret are administrator-owned.
  kubectl --context "$context" -n arc-runners get secret arc-github-credentials >/dev/null
  kubectl --context "$context" apply -f "$deploy_directory/kubernetes/github-actions-role.yaml"
fi

for component in controller scale-set; do
  if [[ "$component" == controller ]]; then
    release=arc
    namespace=arc-system
    chart=gha-runner-scale-set-controller
    values_file="$deploy_directory/runner/controller-values.yaml"
  else
    release=lab4-cloud-runner
    namespace=arc-runners
    chart=gha-runner-scale-set
    values_file="$deploy_directory/runner/scale-set-values.yaml"
  fi
  arguments=("$chart_registry/$chart" --version "$chart_version" \
    --namespace "$namespace" --values "$values_file")
  if [[ "$mode" == template ]]; then
    helm template "$release" "${arguments[@]}"
  else
    helm upgrade --install "$release" "${arguments[@]}" \
      --kube-context "$context" --wait --timeout 5m
  fi
done
