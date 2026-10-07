# PowerX Agent Governance

## Framework delegated Host capability rule

When a Core capability is intentionally consumed by PowerX Framework through a
local Host simulation or a delegated Core binding, it MUST declare both
authorization planes in the same formal platform capability contract:

- An Admin plane, when an admin surface exists: `admin_user` + user JWT + RBAC.
- A service plane: a fixed typed `core_internal` binding with `service_actor`.
- When local Host simulation supports `PX_GATEWAY_API_KEY`, the service binding
  MUST include explicit `api_key` metadata. Core MUST materialize it into an
  `allow_api_key=true`, `api_key_explicit=true` IAM permission and evaluate it
  through published capability + tenant registration + live API-key grant.

Do not use an Admin REST permission as the service grant. Do not grant a broad
`admin_manage` capability merely for a Framework selector; define a
least-privilege `service_read` capability. A `core_internal` capability MUST
use a stable `core://` endpoint, method `INVOKE`, and a typed operation DTO;
it MUST reject arbitrary HTTP method, endpoint, headers, or raw proxy payloads.

Changing this contract is a breaking change: callers using an old free-form
payload must fail explicitly and be migrated to the typed binding. After a
capability change, run `make capability-check`, `make capability-seed`, the
affected API-key/grant tests, and verify `grant-status` for the development
Host API key.
