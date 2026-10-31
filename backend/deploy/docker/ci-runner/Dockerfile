# Deploy/test runner only: no Docker daemon, host socket or AWS credentials.
FROM node:24-bookworm-slim AS node
FROM ghcr.io/actions/actions-runner:2.338.2

USER root
ARG KUBECTL_VERSION=v1.37.1
ARG HELM_VERSION=v4.3.1

COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates curl git jq openssl python3 \
    && rm -rf /var/lib/apt/lists/* \
    && npm install --global newman@6.2.1 \
    && npm cache clean --force

RUN cd /tmp \
    && curl --fail --silent --show-error --location \
      "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" --output kubectl \
    && curl --fail --silent --show-error --location \
      "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl.sha256" --output kubectl.sha256 \
    && printf '%s  kubectl\n' "$(cat kubectl.sha256)" | sha256sum --check \
    && install --mode=0755 kubectl /usr/local/bin/kubectl \
    && curl --fail --silent --show-error --location \
      "https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz" --output helm.tar.gz \
    && curl --fail --silent --show-error --location \
      "https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz.sha256sum" --output helm.sha256sum \
    && expected="$(cut -d ' ' -f 1 helm.sha256sum)" \
    && printf '%s  helm.tar.gz\n' "$expected" | sha256sum --check \
    && tar --extract --gzip --file helm.tar.gz linux-amd64/helm \
    && install --mode=0755 linux-amd64/helm /usr/local/bin/helm \
    && rm -f kubectl kubectl.sha256 helm.tar.gz helm.sha256sum linux-amd64/helm \
    && node --version && newman --version && kubectl version --client && helm version --short

USER runner
