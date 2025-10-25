#!/usr/bin/env bash

set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
	echo "jq is required for this script. Please install jq and retry." >&2
	exit 1
fi

BASE_URL="${POWERX_BASE_URL:-http://localhost:8077/api}"
TOKEN="${POWERX_BEARER_TOKEN:-}"
BASIC_AUTH="${POWERX_BASIC_AUTH:-}"
HTTP_TIMEOUT="${HTTP_TIMEOUT:-10}"

AUTH_HEADERS=()
if [[ -n "${TOKEN}" ]]; then
	AUTH_HEADERS+=("-H" "Authorization: Bearer ${TOKEN}")
elif [[ -n "${BASIC_AUTH}" ]]; then
	AUTH_HEADERS+=("-H" "Authorization: Basic ${BASIC_AUTH}")
fi

CONTENT_HEADERS=(-H "Content-Type: application/json" -H "Accept: application/json")
ACCEPT_HEADER=(-H "Accept: application/json")

CAPABILITY_ID="${CAPABILITY_ID:-capabilities.search.demo}"
TENANT_ID="${TENANT_ID:-tenant-demo}"
PRIMARY_ADAPTER="${PRIMARY_ADAPTER:-search-http-primary}"
PRIMARY_ENDPOINT="${PRIMARY_ENDPOINT:-https://svc-primary/search}"
BACKUP_ADAPTER="${BACKUP_ADAPTER:-search-grpc-backup}"
BACKUP_ENDPOINT="${BACKUP_ENDPOINT:-grpc://svc-backup:8443}"
CLIENT_ID="${CLIENT_ID:-cli-demo}"

HTTP_STATUS=""
RESPONSE=""

log() {
	printf '[%s] %s\n' "$(date +'%Y-%m-%dT%H:%M:%S%z')" "$*"
}

fail() {
	echo "error: $*" >&2
	exit 1
}

call_api() {
	local method=$1
	local url=$2
	local body=${3:-}
	shift 3 || true
	local extra_headers=("$@")

	local response_file
	response_file="$(mktemp)"
	local args=(-sS -o "${response_file}" -w "%{http_code}" --max-time "${HTTP_TIMEOUT}" -X "${method}")
	args+=("${AUTH_HEADERS[@]}" "${extra_headers[@]}")

	if [[ -n "${body}" ]]; then
		args+=("${CONTENT_HEADERS[@]}" "${url}" -d "${body}")
	else
		args+=("${ACCEPT_HEADER[@]}" "${url}")
	fi

	HTTP_STATUS="$(curl "${args[@]}")"
	RESPONSE="$(cat "${response_file}")"
	rm -f "${response_file}"
}

call_api_with_headers() {
	local method=$1
	local url=$2
	local body=${3:-}
	shift 3 || true
	local extra_headers=("$@")

	local response_file header_file
	response_file="$(mktemp)"
	header_file="$(mktemp)"
	local args=(-sS -D "${header_file}" -o "${response_file}" -w "%{http_code}" --max-time "${HTTP_TIMEOUT}" -X "${method}")
	args+=("${AUTH_HEADERS[@]}" "${extra_headers[@]}")

	if [[ -n "${body}" ]]; then
		args+=("${CONTENT_HEADERS[@]}" "${url}" -d "${body}")
	else
		args+=("${ACCEPT_HEADER[@]}" "${url}")
	fi

	HTTP_STATUS="$(curl "${args[@]}")"
	RESPONSE="$(cat "${response_file}")"
	HEADERS="$(cat "${header_file}")"
	rm -f "${response_file}" "${header_file}"
}

assert_status() {
	local expected=$1
	if [[ "${HTTP_STATUS}" != "${expected}" ]]; then
		fail "expected HTTP ${expected}, got ${HTTP_STATUS}: ${RESPONSE}"
	}
}

log "Using API base: ${BASE_URL}"

log "Creating capability registration..."
REGISTRATION_PAYLOAD="$(jq -n \
	--arg capability "${CAPABILITY_ID}" \
	--arg tenant "${TENANT_ID}" \
	--arg contract "contracts/search#v1.2.0" \
	--arg primary "${PRIMARY_ADAPTER}" \
	--arg primaryEndpoint "${PRIMARY_ENDPOINT}" \
	--arg backup "${BACKUP_ADAPTER}" \
	--arg backupEndpoint "${BACKUP_ENDPOINT}" \
	'{
		capability_id: $capability,
		tenant_id: $tenant,
		contract_ref: $contract,
		status: "published",
		adapters: [
			{
				adapter_id: $primary,
				transport_type: "http",
				endpoint_url: $primaryEndpoint,
				weight: 80,
				timeout_ms: 2000
			},
			{
				adapter_id: $backup,
				transport_type: "grpc",
				endpoint_url: $backupEndpoint,
				weight: 20,
				timeout_ms: 2500
			}
		],
		routing_policy: {
			strategy: "priority",
			cooldown_seconds: 60,
			fallback_sequence: [$backup]
		},
		fallback_plan: {
			static_response: {
				payload: {
					message: "capability fallback",
					retry_after_seconds: 30
				},
				ttl_seconds: 60
			}
		},
		tool_grant_ids: []
	}')"
call_api "POST" "${BASE_URL}/admin/capability-registry/capabilities" "${REGISTRATION_PAYLOAD}"
assert_status "201"
VERSION="$(echo "${RESPONSE}" | jq -r '.version')"
log "Capability created with version ${VERSION}"

log "Updating capability weights (optimistic lock)..."
UPDATED_PAYLOAD="$(echo "${REGISTRATION_PAYLOAD}" | jq '.adapters[0].weight = 70 | .adapters[1].weight = 30 | .version = '"${VERSION}"')"
call_api "PUT" "${BASE_URL}/admin/capability-registry/capabilities/${CAPABILITY_ID}/tenants/${TENANT_ID}" "${UPDATED_PAYLOAD}" -H "If-Match: W/\"${VERSION}\""
assert_status "200"
VERSION="$(echo "${RESPONSE}" | jq -r '.version')"
log "Capability updated to version ${VERSION}"

log "Invoking router (expect primary adapter)..."
INVOKE_PAYLOAD="$(jq -n --arg capability "${CAPABILITY_ID}" --arg tenant "${TENANT_ID}" '{capability_id: $capability, tenant_id: $tenant}')"
call_api "POST" "${BASE_URL}/admin/router/invoke" "${INVOKE_PAYLOAD}"
assert_status "200"
PRIMARY_SELECTED="$(echo "${RESPONSE}" | jq -r '.adapter_id')"
log "Router selected adapter: ${PRIMARY_SELECTED}"

log "Marking primary adapter unhealthy..."
HEALTH_BODY="$(jq -n --arg capability "${CAPABILITY_ID}" --arg tenant "${TENANT_ID}" --arg adapter "${PRIMARY_ADAPTER}" '{capability_id: $capability, tenant_id: $tenant, adapter_id: $adapter, status: "unhealthy", reason: "demo-failure"}')"
call_api "POST" "${BASE_URL}/admin/router/health" "${HEALTH_BODY}"
assert_status "202"

log "Marking backup adapter unhealthy to trigger fallback..."
HEALTH_BODY_BACKUP="$(jq -n --arg capability "${CAPABILITY_ID}" --arg tenant "${TENANT_ID}" --arg adapter "${BACKUP_ADAPTER}" '{capability_id: $capability, tenant_id: $tenant, adapter_id: $adapter, status: "unhealthy", reason: "demo-failure"}')"
call_api "POST" "${BASE_URL}/admin/router/health" "${HEALTH_BODY_BACKUP}"
assert_status "202"

log "Invoking router after health degradation (expect fallback response)..."
call_api "POST" "${BASE_URL}/admin/router/invoke" "${INVOKE_PAYLOAD}"
assert_status "200"
FALLBACK_USED="$(echo "${RESPONSE}" | jq -r '.fallback_used')"
if [[ "${FALLBACK_USED}" != "true" ]]; then
	fail "expected fallback to be used; response: ${RESPONSE}"
fi
log "Fallback payload: $(echo "${RESPONSE}" | jq -c '.payload')"

log "Syncing discovery cache..."
SYNC_PAYLOAD="$(jq -n --arg tenant "${TENANT_ID}" --arg capability "${CAPABILITY_ID}" --arg client "${CLIENT_ID}" '{tenant_id: $tenant, capabilities: [$capability], client_id: $client, force: true}')"
call_api "POST" "${BASE_URL}/discovery/sync" "${SYNC_PAYLOAD}"
assert_status "200"
log "Discovery sync completed"

log "Fetching cached snapshot..."
call_api_with_headers "GET" "${BASE_URL}/discovery/${TENANT_ID}/${CAPABILITY_ID}?client_id=${CLIENT_ID}"
assert_status "200"
CACHE_CONTROL="$(printf '%s\n' "${HEADERS}" | awk 'BEGIN{IGNORECASE=1} /^Cache-Control:/ {print $2}' | tr -d '\r')"
if [[ -z "${CACHE_CONTROL}" ]]; then
	CACHE_CONTROL="(missing)"
fi
log "Cache-Control header: ${CACHE_CONTROL}"
log "Snapshot version: $(echo "${RESPONSE}" | jq -r '.version')"

log "Demo completed successfully."
