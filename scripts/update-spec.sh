#!/usr/bin/env sh
# Refreshes spec/openapi.json from the API's published OpenAPI document.
# The coverage tests then say which types need to change.
set -eu
url="${MNML_OPENAPI_URL:-https://api.mnml.ai/v2/openapi.json}"
curl -fsS "$url" -o "$(dirname "$0")/../spec/openapi.json"
echo "spec/openapi.json updated from $url"
