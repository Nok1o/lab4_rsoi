#!/usr/bin/env bash
set -euo pipefail

: "${KUBE_CLUSTER_JSON:?Set KUBE_CLUSTER_JSON to the exported public endpoint/CA JSON}"
: "${YC_IAM_TOKEN:?Set YC_IAM_TOKEN to the temporary OIDC-exchanged IAM token}"
: "${KUBECONFIG:?Set KUBECONFIG to a new temporary file for this CI job}"
if [[ "$KUBECONFIG" != /* || -e "$KUBECONFIG" ]]; then
  echo "KUBECONFIG must be an absolute path to a NEW file." >&2
  exit 2
fi
namespace="${KUBE_NAMESPACE:-library-system}"
# Validate before opening the output; never allow exec/auth-provider/proxy
# configuration supplied through a repository variable into CI's kubeconfig.
jq --exit-status --null-input '
  env.KUBE_CLUSTER_JSON | fromjson |
  (.server | type == "string" and startswith("https://")) and
  (."certificate-authority-data" | type == "string" and length > 0) and
  (."insecure-skip-tls-verify" != true)' >/dev/null
umask 077
set -o noclobber
jq --null-input --arg namespace "$namespace" '
  (env.KUBE_CLUSTER_JSON | fromjson) as $source |
  {
    apiVersion: "v1", kind: "Config",
    clusters: [{name: "rsoi-lab4", cluster: {
      server: $source.server,
      "certificate-authority-data": $source."certificate-authority-data"
    }}],
    users: [{name: "github-actions-oidc", user: {token: env.YC_IAM_TOKEN}}],
    contexts: [{name: "lab4-ci", context: {
      cluster: "rsoi-lab4", user: "github-actions-oidc", namespace: $namespace}}],
    "current-context": "lab4-ci"
  }' > "$KUBECONFIG"
echo "Configured temporary Kubernetes access with certificate verification enabled."
