#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0
#
# Pulls the bike-boxes provider credentials out of the cluster and writes
# them into .env (gitignored) in this directory, for use by
# fetch-rawdata-bicincitta.sh / fetch-rawdata-loewenbytes.sh.
#
# Sources (see infra/helm and .github/workflows for how they got there):
#   - BICINCITTA_CLIENT_ID/SECRET: CI bakes these into the silky config
#     before it's stored as ConfigMap dc-api-crawler-bike-boxes-config's
#     `config.yaml` key (.auth.clientId / .auth.clientSecret) - there is no
#     separate k8s Secret for this provider.
#   - LOEWENBYTES_CLIENT_ID/SECRET: would live in Secret
#     dc-api-crawler-bike-boxes-loewenbytes-env (keys LOEWENBYTES_CLIENT_ID /
#     LOEWENBYTES_CLIENT_SECRET), populated by envSecret in
#     infrastructure/helm/bike-boxes-loewenbytes.*.yaml. As of writing, the
#     deploy jobs in .github/workflows/dc-api-crawler-bike-boxes-loewenbytes.yml
#     are commented out, so this collector has never been deployed and the
#     secret won't exist yet - the script reports this rather than failing
#     silently.
#
# Usage:
#   ./get-secrets-from-k8s.sh [context] [namespace]
#   context   defaults to the current kubectl context
#   namespace defaults to "collector" (matches KUBERNETES_NAMESPACE in the
#             .github/workflows/dc-api-crawler-bike-boxes*.yml deploy jobs)
#
# Requires: kubectl (configured with access to the target cluster), yq

set -uo pipefail
cd "$(dirname "$0")"

CONTEXT="${1:-$(kubectl config current-context 2>/dev/null)}"
NAMESPACE="${2:-collector}"
ENV_FILE=".env"

for bin in kubectl yq; do
  command -v "$bin" >/dev/null || { echo "missing required tool: $bin" >&2; exit 1; }
done

echo "== Using kubectl context '$CONTEXT', namespace '$NAMESPACE' =="

mask() {
  local v="$1"
  [[ -z "$v" ]] && { echo "(empty)"; return; }
  [[ "${#v}" -le 8 ]] && { echo "***"; return; }
  echo "${v:0:4}...${v: -4}"
}

# upsert_env <key> <value>: replace an existing KEY= line in .env, or append it.
upsert_env() {
  local key="$1" value="$2"
  touch "$ENV_FILE"
  if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
    sed -i "s|^${key}=.*|${key}=${value}|" "$ENV_FILE"
  else
    echo "${key}=${value}" >> "$ENV_FILE"
  fi
}

FAILED=0

echo
echo "== bicincitta: reading ConfigMap dc-api-crawler-bike-boxes-config =="
BICINCITTA_CONFIG=$(kubectl --context "$CONTEXT" -n "$NAMESPACE" get configmap dc-api-crawler-bike-boxes-config -o jsonpath='{.data.config\.yaml}' 2>&1)
if [[ $? -ne 0 ]]; then
  echo "  NOT FOUND: $BICINCITTA_CONFIG" >&2
  FAILED=1
else
  BICINCITTA_CLIENT_ID=$(echo "$BICINCITTA_CONFIG" | yq -r '.auth.clientId')
  BICINCITTA_CLIENT_SECRET=$(echo "$BICINCITTA_CONFIG" | yq -r '.auth.clientSecret')
  if [[ -z "$BICINCITTA_CLIENT_ID" || "$BICINCITTA_CLIENT_ID" == "null" ]]; then
    echo "  ConfigMap found but .auth.clientId is empty/null - config may have changed shape" >&2
    FAILED=1
  else
    upsert_env "BICINCITTA_CLIENT_ID" "$BICINCITTA_CLIENT_ID"
    upsert_env "BICINCITTA_CLIENT_SECRET" "$BICINCITTA_CLIENT_SECRET"
    echo "  clientId:     $(mask "$BICINCITTA_CLIENT_ID")"
    echo "  clientSecret: $(mask "$BICINCITTA_CLIENT_SECRET")"
    echo "  Written to $ENV_FILE"
  fi
fi

echo
echo "== loewenbytes: reading Secret dc-api-crawler-bike-boxes-loewenbytes-env =="
LB_ID=$(kubectl --context "$CONTEXT" -n "$NAMESPACE" get secret dc-api-crawler-bike-boxes-loewenbytes-env -o jsonpath='{.data.LOEWENBYTES_CLIENT_ID}' 2>&1)
if [[ $? -ne 0 ]]; then
  echo "  NOT FOUND: $LB_ID" >&2
  echo "  The dc-api-crawler-bike-boxes-loewenbytes collector has never been deployed" >&2
  echo "  (its deploy jobs are commented out in" >&2
  echo "  .github/workflows/dc-api-crawler-bike-boxes-loewenbytes.yml), so this Secret" >&2
  echo "  doesn't exist in the cluster yet. The credentials only exist as GitHub Actions" >&2
  echo "  secrets (LOEWENBYTES_CLIENT_ID_TEST/PROD) or wherever the loewenbytes-onboarding" >&2
  echo "  contact shared them - not fetchable via kubectl." >&2
  FAILED=1
else
  LB_SECRET=$(kubectl --context "$CONTEXT" -n "$NAMESPACE" get secret dc-api-crawler-bike-boxes-loewenbytes-env -o jsonpath='{.data.LOEWENBYTES_CLIENT_SECRET}' | base64 -d)
  LB_ID=$(echo "$LB_ID" | base64 -d)
  upsert_env "LOEWENBYTES_CLIENT_ID" "$LB_ID"
  upsert_env "LOEWENBYTES_CLIENT_SECRET" "$LB_SECRET"
  echo "  clientId:     $(mask "$LB_ID")"
  echo "  clientSecret: $(mask "$LB_SECRET")"
  echo "  Written to $ENV_FILE"
fi

echo
if [[ "$FAILED" -ne 0 ]]; then
  echo "Done with warnings - see above. $ENV_FILE updated for whatever was found."
  exit 1
fi
echo "Done. $ENV_FILE updated with both providers' credentials."
