#!/usr/bin/env bash
#
# run_local_tunnel.sh - Set up and run local Cloudflare Tunnel for ONDC local deployment.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Configuration defaults
CONFIG_TEMPLATE="${WORKSPACE_ROOT}/config/cloudflared/config.yaml.template"
OUTPUT_CONFIG_DIR="${WORKSPACE_ROOT}/tmp/cloudflared"
OUTPUT_CONFIG="${OUTPUT_CONFIG_DIR}/config.yaml"

TUNNEL_ID="${TUNNEL_ID:-test-tunnel-id}"
TUNNEL_HOSTNAME="${TUNNEL_HOSTNAME:-ondc-local.example.com}"
CREDENTIALS_FILE="${CREDENTIALS_FILE:-${OUTPUT_CONFIG_DIR}/${TUNNEL_ID}.json}"
ONBOARDING_PORT="${ONBOARDING_PORT:-8080}"
BAP_PORT="${BAP_PORT:-8081}"
BPP_PORT="${BPP_PORT:-8082}"

print_usage() {
  cat <<EOF
Usage: $(basename "$0") [command]

Commands:
  generate-config   Generate rendered Cloudflare Tunnel configuration file from template.
  validate-config   Validate generated configuration with cloudflared ingress rule checker.
  run-tunnel        Launch cloudflared tunnel using the generated configuration.
  check-deps        Verify required binaries (cloudflared, bash).
  help              Display this help message.

Environment Variables:
  TUNNEL_ID         Cloudflare Tunnel UUID or name (default: test-tunnel-id)
  TUNNEL_HOSTNAME   Public hostname routed to this tunnel (default: ondc-local.example.com)
  CREDENTIALS_FILE  Path to Cloudflare credentials json file
  ONBOARDING_PORT   Local port for Onboarding service (default: 8080)
  BAP_PORT          Local port for BAP API (default: 8081)
  BPP_PORT          Local port for BPP API (default: 8082)
EOF
}

check_deps() {
  if ! command -v cloudflared >/dev/null 2>&1; then
    echo "ERROR: 'cloudflared' command not found in PATH." >&2
    exit 1
  fi
  echo "cloudflared binary found: $(which cloudflared)"
}

generate_config() {
  mkdir -p "${OUTPUT_CONFIG_DIR}"

  # Substitute template variables
  sed -e "s|\${TUNNEL_ID}|${TUNNEL_ID}|g" \
      -e "s|\${TUNNEL_HOSTNAME}|${TUNNEL_HOSTNAME}|g" \
      -e "s|\${CREDENTIALS_FILE}|${CREDENTIALS_FILE}|g" \
      -e "s|\${ONBOARDING_PORT:-8080}|${ONBOARDING_PORT}|g" \
      -e "s|\${BAP_PORT:-8081}|${BAP_PORT}|g" \
      -e "s|\${BPP_PORT:-8082}|${BPP_PORT}|g" \
      "${CONFIG_TEMPLATE}" > "${OUTPUT_CONFIG}"

  echo "Generated Cloudflare Tunnel configuration at: ${OUTPUT_CONFIG}"
}

validate_config() {
  if [ ! -f "${OUTPUT_CONFIG}" ]; then
    generate_config
  fi

  check_deps

  echo "Validating Cloudflare Tunnel ingress rules..."
  # cloudflared tunnel [--config FILEPATH] ingress validate
  cloudflared tunnel --config "${OUTPUT_CONFIG}" ingress validate
  echo "Cloudflare Tunnel configuration is valid."
}

run_tunnel() {
  if [ ! -f "${OUTPUT_CONFIG}" ]; then
    generate_config
  fi

  check_deps

  echo "Starting Cloudflare Tunnel for ${TUNNEL_HOSTNAME}..."
  exec cloudflared tunnel --config "${OUTPUT_CONFIG}" run "${TUNNEL_ID}"
}

case "${1:-validate-config}" in
  generate-config)
    generate_config
    ;;
  validate-config)
    validate_config
    ;;
  run-tunnel)
    run_tunnel
    ;;
  check-deps)
    check_deps
    ;;
  help|--help|-h)
    print_usage
    ;;
  *)
    echo "Unknown command: $1" >&2
    print_usage
    exit 1
    ;;
esac
