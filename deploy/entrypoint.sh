#!/usr/bin/env bash
set -euo pipefail

load_cloud_tools_or_exit() {
  local tools_file="/root/.sealos/cloud/scripts/tools.sh"
  local required_functions=(
    get_cm_value
    global_http_disable_https
    global_http_effective_port
    global_http_external_url
    info
    warn
    error
  )
  local missing_functions=()
  local function_name

  if [ ! -f "$tools_file" ]; then
    cat >&2 <<'EOF'
错误：未找到 /root/.sealos/cloud/scripts/tools.sh，当前组件镜像无法继续执行。

请先回到当前安装包目录，执行对应命令同步 values + tools：
  Pro 安装包：./sealos-pro.sh sync-config
  OSS 安装包：./sealos-oss.sh sync-config
EOF
    exit 1
  fi

  # shellcheck source=/dev/null
  source /root/.sealos/cloud/scripts/tools.sh

  for function_name in "${required_functions[@]}"; do
    if ! declare -f "$function_name" >/dev/null 2>&1; then
      missing_functions+=("$function_name")
    fi
  done

  if [ "${#missing_functions[@]}" -gt 0 ]; then
    cat >&2 <<EOF
错误：/root/.sealos/cloud/scripts/tools.sh 版本过旧，缺少配置检测函数，当前组件镜像无法继续执行。

缺少函数：${missing_functions[*]}

请先回到当前安装包目录，执行对应命令同步 values + tools：
  Pro 安装包：./sealos-pro.sh sync-config
  OSS 安装包：./sealos-oss.sh sync-config
EOF
    exit 1
  fi

}

append_app_values() {
  local values_dir="$1"
  local values_file

  if [ ! -d "$values_dir" ]; then
    return 0
  fi

  while IFS= read -r values_file; do
    [ -n "$values_file" ] || continue
    HELM_VALUES_ARGS+=("-f" "$values_file")
  done < <(find "$values_dir" -maxdepth 1 -type f -name '*-values.yaml' | sort)
}

sync_packaged_app_values() {
  local packaged_values_file="$1"
  local app_values_dir="$2"

  if [ -d "$app_values_dir" ]; then
    return 0
  fi

  warn "WARN: app values dir missing; initializing ${app_values_dir} from packaged values ${packaged_values_file}"
  mkdir -p "$app_values_dir"
  cp -f "$packaged_values_file" "${app_values_dir}/storage-manager-values.yaml"
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

RELEASE_NAME=${RELEASE_NAME:-"storage-manager"}
RELEASE_NAMESPACE=${RELEASE_NAMESPACE:-"storage-manager"}
CHART_PATH=${CHART_PATH:-"${SCRIPT_DIR}/charts/storage-manager"}
PACKAGED_APP_VALUES_FILE=${PACKAGED_APP_VALUES_FILE:-"${CHART_PATH}/storage-manager-values.yaml"}
APP_VALUES_DIR=${APP_VALUES_DIR:-"/root/.sealos/cloud/values/apps/storage-manager"}
SEALOS_SYSTEM_NS=${SEALOS_SYSTEM_NS:-"sealos-system"}
SEALOS_CONFIG_CM=${SEALOS_CONFIG_CM:-"sealos-config"}
HELM_OPTS=${HELM_OPTS:-""}

[ -d "$CHART_PATH" ] || {
  echo "chart directory not found: ${CHART_PATH}" >&2
  exit 1
}

[ -f "$PACKAGED_APP_VALUES_FILE" ] || {
  echo "packaged values file not found: ${PACKAGED_APP_VALUES_FILE}" >&2
  exit 1
}

load_cloud_tools_or_exit

for cmd in helm kubectl find sort mkdir cp; do
  command -v "$cmd" >/dev/null 2>&1 || error "missing required command: ${cmd}"
done

CLOUD_DOMAIN="${SEALOS_CLOUD_DOMAIN:-}"
if [ -z "$CLOUD_DOMAIN" ]; then
  CLOUD_DOMAIN="$(get_cm_value "$SEALOS_SYSTEM_NS" "$SEALOS_CONFIG_CM" cloudDomain 1 0)"
fi
[ -n "$CLOUD_DOMAIN" ] || error "missing required ${SEALOS_SYSTEM_NS}/${SEALOS_CONFIG_CM} cloudDomain"

SEALOS_CLOUD_PORT="${SEALOS_CLOUD_PORT:-}"
if [ -z "$SEALOS_CLOUD_PORT" ]; then
  SEALOS_CLOUD_PORT="$(get_cm_value "$SEALOS_SYSTEM_NS" "$SEALOS_CONFIG_CM" cloudPort 1 0)"
fi

SEALOS_HTTP_PORT="${SEALOS_HTTP_PORT:-}"
if [ -z "$SEALOS_HTTP_PORT" ]; then
  SEALOS_HTTP_PORT="$(get_cm_value "$SEALOS_SYSTEM_NS" "$SEALOS_CONFIG_CM" httpPort 1 0)"
fi

SEALOS_DISABLE_HTTPS="${SEALOS_DISABLE_HTTPS:-}"
if [ -z "$SEALOS_DISABLE_HTTPS" ]; then
  SEALOS_DISABLE_HTTPS="$(get_cm_value "$SEALOS_SYSTEM_NS" "$SEALOS_CONFIG_CM" disableHttps 1 0)"
fi
export SEALOS_CLOUD_PORT SEALOS_HTTP_PORT SEALOS_DISABLE_HTTPS

if global_http_disable_https; then
  SEALOS_DISABLE_HTTPS="true"
else
  SEALOS_DISABLE_HTTPS="false"
fi
export SEALOS_DISABLE_HTTPS

SEALOS_CERT_SECRET_NAME="${SEALOS_CERT_SECRET_NAME:-}"
if [ -z "$SEALOS_CERT_SECRET_NAME" ]; then
  SEALOS_CERT_SECRET_NAME="$(get_cm_value "$SEALOS_SYSTEM_NS" "$SEALOS_CONFIG_CM" certSecretName 1 0)"
fi
SEALOS_CERT_SECRET_NAME="${SEALOS_CERT_SECRET_NAME:-wildcard-cert}"

WEB_HOST="${WEB_HOST:-storage-manager.${CLOUD_DOMAIN}}"
WEB_URL="$(global_http_external_url "$WEB_HOST")"
EFFECTIVE_PORT="$(global_http_effective_port)"

sync_packaged_app_values "$PACKAGED_APP_VALUES_FILE" "$APP_VALUES_DIR"

HELM_VALUES_ARGS=()
append_app_values "$APP_VALUES_DIR"

HELM_COMMON_ARGS=(
  "--set-string" "cloudDomain=${CLOUD_DOMAIN}"
  "--set-string" "cloudPort=${SEALOS_CLOUD_PORT}"
  "--set-string" "httpPort=${SEALOS_HTTP_PORT}"
  "--set-string" "disableHttps=${SEALOS_DISABLE_HTTPS}"
  "--set-string" "certSecretName=${SEALOS_CERT_SECRET_NAME}"
)

info "Preparing release=${RELEASE_NAME}, namespace=${RELEASE_NAMESPACE}, chart=${CHART_PATH}"
info "Storage Manager URL=${WEB_URL}, disableHttps=${SEALOS_DISABLE_HTTPS}, effectivePort=${EFFECTIVE_PORT}"
info "Synced packaged values=${PACKAGED_APP_VALUES_FILE}; using app values dir=${APP_VALUES_DIR}"

# shellcheck disable=SC2086
helm upgrade -i "${RELEASE_NAME}" "${CHART_PATH}" -n "${RELEASE_NAMESPACE}" --create-namespace \
  "${HELM_VALUES_ARGS[@]}" \
  "${HELM_COMMON_ARGS[@]}" \
  ${HELM_OPTS}
