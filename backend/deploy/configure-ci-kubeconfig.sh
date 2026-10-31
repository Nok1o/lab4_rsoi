#!/usr/bin/env bash
set -euo pipefail

: "${KUBECONFIG:?Set KUBECONFIG to a new temporary file for this CI job}"
if [[ "$KUBECONFIG" != /* || -e "$KUBECONFIG" || -L "$KUBECONFIG" ]]; then
  echo "KUBECONFIG must be an absolute path to a NEW file." >&2
  exit 2
fi

service_account_directory="${KUBE_SERVICE_ACCOUNT_DIRECTORY:-/var/run/secrets/kubernetes.io/serviceaccount}"
if [[ "$service_account_directory" != /* ]]; then
  echo "ServiceAccount mount directory must be an absolute path." >&2
  exit 2
fi
for file in ca.crt token; do
  if [[ ! -r "$service_account_directory/$file" || ! -s "$service_account_directory/$file" ]]; then
    echo "Missing mounted ServiceAccount $file; run this job in the ARC runner Pod." >&2
    exit 2
  fi
done

umask 077
set -o noclobber
# Reference the projected token FILE, not a snapshot: kubelet rotates the token
# during long-running jobs. This also works when Classroom drops the job env.
jq --null-input --arg directory "$service_account_directory" \
  --arg namespace "${KUBE_NAMESPACE:-library-system}" '
  {
    apiVersion: "v1", kind: "Config",
    clusters: [{name: "in-cluster", cluster: {
      server: "https://kubernetes.default.svc:443",
      "certificate-authority": ($directory + "/ca.crt")
    }}],
    users: [{name: "github-actions", user: {
      tokenFile: ($directory + "/token")
    }}],
    contexts: [{name: "lab4-ci", context: {
      cluster: "in-cluster", user: "github-actions", namespace: $namespace}}],
    "current-context": "lab4-ci"
  }' > "$KUBECONFIG"
echo "Configured in-cluster access using the runner ServiceAccount and verified TLS."
