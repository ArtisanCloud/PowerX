# PowerX Agent Governance

## 调试服务进程管理

- Agent 可以为开发、联调和验收临时启动服务，启动时必须记录进程 PID、启动命令、工作目录及监听端口，明确进程归属。
- 调试结束后、向用户交付前，必须关闭 Agent 自己启动的服务及其子进程，并确认进程已经退出、相关监听端口已经释放。只有用户明确要求保留运行时，才可以继续运行。
- 日常服务由用户自行启动。需要重新编译或重启时，告知用户具体操作，不得在调试结束后遗留临时服务进程。
- 不得擅自关闭或重启用户启动的进程；清理前必须核实 PID 和进程归属。

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
