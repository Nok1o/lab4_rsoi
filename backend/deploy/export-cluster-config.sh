#!/usr/bin/env bash
set -euo pipefail

# Read local kubeconfig only. Do not export users, tokens, exec plugins or keys.
context="${KUBE_CONTEXT:-$(kubectl config current-context)}"
kubectl --context "$context" config view --minify --flatten --raw -o json |
  jq --exit-status '
    .clusters[0].cluster |
    if (.server | type) != "string" or
       (.server | startswith("https://") | not) or
       (."certificate-authority-data" | type) != "string" or
       (."certificate-authority-data" | length) == 0 or
       ."insecure-skip-tls-verify" == true then
      error("An HTTPS endpoint and embedded CA are required; TLS verification must stay enabled")
    else {server, "certificate-authority-data": ."certificate-authority-data"} end'
