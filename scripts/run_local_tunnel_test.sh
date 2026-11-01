#!/usr/bin/env bash
#
# run_local_tunnel_test.sh - Unit & integration tests for Cloudflare Tunnel scripts and templates.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

FAILED=0

test_check_deps() {
  echo "RUN  test_check_deps"
  if "${WORKSPACE_ROOT}/scripts/run_local_tunnel.sh" check-deps >/dev/null 2>&1; then
    echo "PASS test_check_deps"
  else
    echo "FAIL test_check_deps: cloudflared dependency check failed"
    FAILED=$((FAILED + 1))
  fi
}

test_generate_and_validate_config() {
  echo "RUN  test_generate_and_validate_config"
  export TUNNEL_ID="test-ondc-tunnel-uuid"
  export TUNNEL_HOSTNAME="dev.ondc.example.internal"
  export ONBOARDING_PORT="18080"
  export BAP_PORT="18081"
  export BPP_PORT="18082"

  if "${WORKSPACE_ROOT}/scripts/run_local_tunnel.sh" generate-config >/dev/null; then
    CONFIG_PATH="${WORKSPACE_ROOT}/tmp/cloudflared/config.yaml"
    if [ ! -f "${CONFIG_PATH}" ]; then
      echo "FAIL test_generate_and_validate_config: config file was not created"
      FAILED=$((FAILED + 1))
      return
    fi

    # Check variable substitution in generated file
    if ! grep -q "tunnel: test-ondc-tunnel-uuid" "${CONFIG_PATH}"; then
      echo "FAIL test_generate_and_validate_config: TUNNEL_ID not found in config"
      FAILED=$((FAILED + 1))
    fi
    if ! grep -q "hostname: dev.ondc.example.internal" "${CONFIG_PATH}"; then
      echo "FAIL test_generate_and_validate_config: TUNNEL_HOSTNAME not found in config"
      FAILED=$((FAILED + 1))
    fi
    if ! grep -q "http://localhost:18080" "${CONFIG_PATH}"; then
      echo "FAIL test_generate_and_validate_config: ONBOARDING_PORT not substituted"
      FAILED=$((FAILED + 1))
    fi
    if ! grep -q "http://localhost:18081" "${CONFIG_PATH}"; then
      echo "FAIL test_generate_and_validate_config: BAP_PORT not substituted"
      FAILED=$((FAILED + 1))
    fi
    if ! grep -q "http://localhost:18082" "${CONFIG_PATH}"; then
      echo "FAIL test_generate_and_validate_config: BPP_PORT not substituted"
      FAILED=$((FAILED + 1))
    fi

    # Validate syntax with cloudflared
    if "${WORKSPACE_ROOT}/scripts/run_local_tunnel.sh" validate-config >/dev/null; then
      echo "PASS test_generate_and_validate_config"
    else
      echo "FAIL test_generate_and_validate_config: ingress validation failed"
      FAILED=$((FAILED + 1))
    fi
  else
    echo "FAIL test_generate_and_validate_config: failed to run generate-config"
    FAILED=$((FAILED + 1))
  fi
}

test_ingress_routes_completeness() {
  echo "RUN  test_ingress_routes_completeness"
  CONFIG_PATH="${WORKSPACE_ROOT}/tmp/cloudflared/config.yaml"

  # Ensure routes for /ondc-site-verification.html, /onboarding/*, /bap/*, /bpp/* and catch-all exist
  required_routes=(
    "/ondc-site-verification.html"
    "/onboarding/*"
    "/bap/*"
    "/bpp/*"
    "http_status:404"
  )

  for route in "${required_routes[@]}"; do
    if ! grep -q "${route}" "${CONFIG_PATH}"; then
      echo "FAIL test_ingress_routes_completeness: missing required route ${route}"
      FAILED=$((FAILED + 1))
    fi
  done
  echo "PASS test_ingress_routes_completeness"
}

# Run tests
test_check_deps
test_generate_and_validate_config
test_ingress_routes_completeness

if [ "${FAILED}" -eq 0 ]; then
  echo "ALL TESTS PASSED"
  exit 0
else
  echo "${FAILED} TESTS FAILED"
  exit 1
fi
