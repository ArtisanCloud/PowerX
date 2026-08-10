# Feature Specification: IAM 用户与角色 RBAC 统一能力

**Feature Branch**: `026-iam`  
**Created**: 2026-04-06  
**Status**: Draft  
**Domain Ownership**: CoreX (`corex.iam`, `corex.rbac`)  
**Input**: User description: "IAM 用户与角色 RBAC 统一能力：补齐 root/tenant admin/member 权限模型、/settings/users 交互语义、me/context 状态一致性与验收标准，产出 specs/026-iam 规范"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 角色边界清晰可执行 (Priority: P1)

作为平台管理员或租户管理员，我希望系统对 root、租户管理员、普通成员的权限边界清晰且可验证，以避免越权或误判权限。

**Why this priority**: 权限边界是 IAM/RBAC 的核心，边界不清会直接导致安全风险和运营混乱。

**Independent Test**: 分别使用 root、租户管理员、普通成员账号登录，验证三类角色在“用户管理”中的可见范围与可执行操作是否符合预期。

**Acceptance Scenarios**:

1. **Given** root 账号已登录，**When** 进入用户管理模块，**Then** 可查看并管理所有租户用户。
2. **Given** 租户管理员账号已登录，**When** 进入用户管理模块，**Then** 仅可查看并管理所属租户用户，无法跨租户管理。
3. **Given** 普通成员账号已登录，**When** 进入用户管理模块，**Then** 仅可查看受限信息，不能执行租户级用户管理操作。

---

### User Story 2 - 用户管理交互语义一致 (Priority: P1)

作为用户管理页面使用者，我希望“查看详情”“切换租户上下文”“进入其他业务页”是明确分离的动作，避免误触导致上下文变化或页面跳转。

**Why this priority**: 当前误解与误操作主要来自交互语义混杂，影响日常管理效率和信任。

**Independent Test**: 在用户管理页面逐一执行“点击租户记录”“切换租户”“进入其他页”动作，验证每个动作结果单一且可预期。

**Acceptance Scenarios**:

1. **Given** 用户位于租户列表，**When** 点击某租户记录，**Then** 系统仅进入该租户详情/用户列表视图，不隐式跳转其他页面。
2. **Given** 用户需要切换租户上下文，**When** 触发“切换租户”独立动作，**Then** 系统重新签发指向目标租户成员的 token/context，不执行无关跳转。
3. **Given** 用户需要进入其他业务页，**When** 触发“进入仪表盘”等独立动作，**Then** 系统按动作目标跳转且不改变未声明的上下文。

---

### User Story 3 - 上下文状态强一致 (Priority: P2)

作为管理员，我希望页面视图与服务端身份上下文始终一致，即使存在缓存、刷新、跨标签页操作，也不会出现“明明是 root 却显示只读视图”的状态错位。

**Why this priority**: 状态错位会造成高频误报与排障成本，且影响权限功能可信度。

**Independent Test**: 在登录后、强刷后、跨标签页切换后重复进入用户管理模块，验证页面角色视图始终与服务端上下文一致。

**Acceptance Scenarios**:

1. **Given** 用户身份与租户上下文已在服务端确定，**When** 进入用户管理模块，**Then** 页面使用最新上下文进行角色分流展示。
2. **Given** 本地缓存与服务端上下文不一致，**When** 页面加载关键管理视图，**Then** 以服务端上下文为准并完成本地状态纠正。
3. **Given** 会话已失效或上下文异常，**When** 请求当前身份上下文，**Then** 系统返回可识别错误并引导到统一会话恢复路径。
4. **Given** 同一 user 加入多个租户，**When** 使用邮箱或手机号登录且未显式选择租户，**Then** 系统选择最近使用且仍有效的租户；若最近租户无效，则选择第一个 active member。
5. **Given** 登录请求显式传入目标租户，**When** 该 user 拥有目标租户 active member，**Then** token/context 指向该租户成员；无权限或无效时不得越权使用该租户。

---

### User Story 4 - 新租户管理员路径可验证 (Priority: P3)

作为新租户注册用户，我希望注册后能以租户管理员身份管理本租户成员，且权限不扩散到其他租户。

**Why this priority**: 这是多租户自服务增长场景的基础能力，但优先级低于平台权限边界与管理体验收敛。

**Independent Test**: 创建新租户并使用注册账号登录，验证其可管理本租户成员且无法执行跨租户操作。

**Acceptance Scenarios**:

1. **Given** 新租户注册完成，**When** 注册账号登录并进入用户管理模块，**Then** 可管理本租户成员。
2. **Given** 新租户管理员尝试访问其他租户成员数据，**When** 发起查询或管理动作，**Then** 系统拒绝并不泄露目标租户信息。

---

### User Story 5 - SaaS 自助开通租户 (Priority: P1)

作为 SaaS 新用户，我希望通过公开注册入口创建自己的租户，并自动成为该租户 owner/admin，以便不用平台 root 手工初始化租户。

**Why this priority**: SaaS 模式的核心入口是 tenant bootstrap；没有这个能力，多租户仍然依赖平台后台手动开通。

**Independent Test**: 使用新邮箱注册新租户，验证 tenant、user、member、role binding、默认租户设置在一个事务语义下创建完成，且登录后上下文指向新租户。

**Acceptance Scenarios**:

1. **Given** 新邮箱/手机号和租户名称，**When** 调用 SaaS signup，**Then** 系统按租户名称生成唯一 tenant key，或使用用户显式填写且未冲突的 tenant key，创建 tenant、user、member，并绑定 `role_owner`、`role_admin`、`role_user`。
2. **Given** 已有邮箱且密码正确，**When** 调用 SaaS signup 创建第二租户，**Then** 系统复用 user 并创建新的 tenant member。
3. **Given** 已有邮箱但密码错误，**When** 调用 SaaS signup，**Then** 系统拒绝创建租户。
4. **Given** tenant key 已存在，**When** 调用 SaaS signup，**Then** 系统拒绝并不留下半成品 tenant。

---

### User Story 6 - Root 平台身份与租户身份隔离 (Priority: P1)

作为平台 root，我希望默认只进入平台控制台，而不是被自动当成某个租户管理员，以避免误操作租户业务配置。

**Why this priority**: root 与租户 admin 混用会直接破坏 SaaS 权限边界，尤其影响 AI Settings、插件启用、业务数据页面。

**Independent Test**: 使用 root 登录，验证默认菜单、AI Settings、租户插件业务页、租户切换行为都符合平台身份边界。

**Acceptance Scenarios**:

1. **Given** root 已登录，**When** 打开默认后台，**Then** 进入 Platform Console，而不是 Tenant Console。
2. **Given** root 未进入 Support Session，**When** 访问租户 AI Settings，**Then** 系统拒绝或隐藏入口。
3. **Given** root 未进入 Support Session，**When** 访问租户插件业务页面，**Then** 系统拒绝或隐藏入口。
4. **Given** root 需要查看某租户，**When** 创建 Support Session，**Then** 系统记录 target tenant、reason、actor 和审计事件。

---

### User Story 7 - 租户插件实例隔离 (Priority: P2)

作为租户 owner/admin，我希望启用插件只影响当前租户，不会重装或删除其他租户正在使用的插件包。

**Why this priority**: 当前插件物理安装目录是全局版本维度，SaaS 场景必须拆分全局插件包与租户插件实例，否则一个租户操作会影响其他租户。

**Independent Test**: 租户 A 启用插件、租户 B 不启用；分别访问菜单、插件 admin、插件 api，验证租户隔离生效。

**Acceptance Scenarios**:

1. **Given** 租户 A 启用插件且租户 B 未启用，**When** 两个租户加载菜单，**Then** 只有租户 A 看到插件菜单。
2. **Given** 租户 B 未启用插件，**When** 直接访问 `/_p/<plugin>/admin`，**Then** 系统拒绝。
3. **Given** 租户 B 未启用插件，**When** 直接访问 `/_p/<plugin>/api`，**Then** 系统拒绝。
4. **Given** 租户 A 停用插件，**When** 租户 B 使用同一插件包，**Then** 租户 B 不受影响。
5. **Given** 租户 A 和租户 B 都启用同一插件，**When** 查看 PowerX 节点内存运行时，**Then** 同一插件只存在一组全局运行进程，而不是每租户一组进程。
6. **Given** 插件进程收到来自不同租户的请求，**When** 处理业务逻辑，**Then** 插件必须从当前请求/事件上下文识别 tenant/member，而不是使用进程启动时固定租户。
7. **Given** 任意租户仍存在目标插件实例，**When** root 直接卸载全局插件包，**Then** 系统必须拒绝同步卸载并要求进入 drain 流程。
8. **Given** root 对目标插件发起 drain，**When** 某租户实例仍有活跃 session、API 写入、scheduler job、queue task 或插件 DrainStatus 未 ready，**Then** 该租户实例不得被标记为 drained。
9. **Given** root 执行同版本 replace，**When** 替换完成，**Then** 租户插件实例、订阅、配置、权限和业务数据不得被删除。

---

### User Story 8 - 历史数据语义迁移可控 (Priority: P2)

作为平台运维人员，我希望 SaaS IAM 语义上线时不破坏已有组织架构、root 安装记录和租户成员关系，并能通过巡检发现需要补齐的数据。

**Why this priority**: 服务器已有组织架构数据不能靠人工删改；迁移必须先巡检、再自动补齐可推导数据，并明确无法推导的数据。

**Independent Test**: 在已有数据环境执行 IAM 巡检，验证 root、system tenant、业务租户 owner/admin 缺失情况可见，且自动补齐不会破坏部门和角色关系。

**Acceptance Scenarios**:

1. **Given** 已有 root user 和 `system` tenant member，**When** 执行迁移，**Then** 两者不会被删除或重建。
2. **Given** 业务租户缺少 `role_owner` 但存在 `role_admin`，**When** 执行补齐迁移，**Then** 系统把最早 admin 补为 owner 并写审计。
3. **Given** 业务租户没有 active admin，**When** 执行巡检，**Then** 系统只报告异常，不自动猜测 owner。
4. **Given** 已有部门树和成员部门关系，**When** 执行迁移后登录，**Then** 组织架构显示不变。

---

### User Story 9 - 角色级菜单权限控制 (Priority: P1)

作为租户管理员，我希望左侧菜单按角色授权显示，而不是所有成员都看到全部 Pinned 菜单，以便不同岗位只看到其被授权使用的后台入口。

**Why this priority**: 菜单是后台功能的第一层入口。若菜单不受角色权限控制，会造成越权感知、误操作和排障混乱；但菜单隐藏不等于 API 授权，目标页面和接口仍必须独立校验权限。

**Independent Test**: 给同一租户下两个普通成员绑定不同角色，分别登录并请求 `/api/v1/admin/menus`，验证返回菜单只包含该角色拥有的 `menu:*:read` 权限项。

**Acceptance Scenarios**:

1. **Given** 成员只绑定 `role_user`，**When** 加载后台菜单，**Then** 只显示该角色拥有的菜单权限入口，不显示未授权 Pinned 项。
2. **Given** 成员绑定自定义角色且该角色拥有 `menu:workflow:read`，**When** 加载后台菜单，**Then** 显示工作流菜单。
3. **Given** 成员没有 `menu:skills:read`，**When** 加载后台菜单，**Then** 不显示 Skill 管理入口。
4. **Given** root 登录，**When** 加载后台菜单，**Then** root 可看到平台授权入口，且仍受 root/tenant 身份边界约束。
5. **Given** 供应商成员绑定 `role_vendor`，**When** 加载后台菜单，**Then** 默认只看到供应商默认入口，例如 Dashboard、Agent 对话和插件智能体对话入口。
6. **Given** 插件声明了 `frontend.admin.menus` 且已安装/启用，**When** 插件 manifest 权限同步完成，**Then** 系统自动生成 `module=menu, resource=plugin.<plugin_id>.<menu_id>, action=read` 的插件菜单权限，管理员可在角色权限页授权该 App 菜单。
7. **Given** 某角色未拥有目标插件菜单权限，**When** 请求 `/api/v1/admin/menus`，**Then** 即使租户已启用该插件，也不返回该插件菜单入口。
8. **Given** 插件通过 Capability Sync 声明了 `menu/page/action/api` 细颗粒度权限，**When** 租户管理员进入角色权限页，**Then** 页面按插件、模块、菜单、页面、动作分组展示可授权项，角色绑定同一 `permission_code`，接口 binding 只作为 enforcement 元数据。
9. **Given** 角色未拥有插件 action 权限，**When** 用户进入插件业务页，**Then** 对应按钮不展示；若直接调用绑定接口，Gateway 和插件后端都必须按同一 `permission_code` 拒绝。

---

### User Story 10 - 租户注册准入与灰度开放 (Priority: P1)

作为 PowerX SaaS 平台 root，我希望在 setup 安装阶段和 root 后台统一配置租户注册准入策略，以便在正式上线前支持关闭注册、邀请制、候补名单、人工审核、白名单、灰度放量和最终完全开放。

**Why this priority**: SaaS signup 是租户增长入口。单一 `enable_saas_signup` 布尔开关无法支撑内测、分批放量、邀请码、每日配额、审核和紧急关闭；若没有后端强制策略，前端隐藏按钮也无法形成安全边界。

**Independent Test**: 依次激活 `closed`、`invite_only`、`waitlist`、`approval_required`、`allowlist`、`progressive_rollout`、`open` 策略，分别调用 effective policy、验证码发送、signup 和 root 审核接口，验证准入结果、租户创建、申请记录、邀请码消耗、审计事件和回滚行为符合策略。

**Acceptance Scenarios**:

1. **Given** 当前 active 策略为 `closed`，**When** 用户请求验证码或提交 signup，**Then** 系统拒绝并返回机器可读错误码 `registration_closed`，且不创建 tenant/user/member。
2. **Given** 当前 active 策略为 `open`，**When** 用户提交符合基础校验的 signup，**Then** 系统按 US5 事务语义创建 tenant、owner user/member 和默认角色，并记录准入审计。
3. **Given** 当前 active 策略为 `invite_only`，**When** signup 未提交有效结构化 `invite_code`，**Then** 系统拒绝；**When** invite code 有效，**Then** 邀请码消耗与租户创建在同一事务中完成。
4. **Given** 当前 active 策略为 `waitlist`，**When** 用户提交注册申请，**Then** 系统仅创建 `registration_request`，不创建 tenant/user/member。
5. **Given** 当前 active 策略为 `approval_required`，**When** 用户提交完整开户资料，**Then** 系统创建待审核申请；**When** root 审核通过，**Then** 才执行租户创建事务。
6. **Given** 当前 active 策略为 `allowlist`，**When** 用户 contact、邮箱域名或渠道未命中白名单，**Then** 系统明确拒绝并记录 `reason_code`；命中时才继续注册链路。
7. **Given** 当前 active 策略为 `progressive_rollout`，**When** 用户被稳定百分比、时间窗口、配额或批次规则命中，**Then** 系统返回可解释准入结果；同一 contact 在同一策略版本下的百分比命中结果必须稳定。
8. **Given** root 在后台切换策略，**When** 策略激活，**Then** 系统生成新策略版本并归档旧版本，所有后续准入审计必须记录策略 UUID 和版本。
9. **Given** 缺少 active 注册策略、策略 mode 未知或规则 type 未知，**When** 请求 effective policy、验证码或 signup，**Then** 系统 fail fast，不回退到旧布尔配置或默认开放。

---

### Edge Cases

- root 账号仅存在一个系统租户成员关系时，仍需保持其跨租户管理能力表达清晰。
- 租户管理员被降权或禁用后，已打开页面应在可接受时间内收敛到受限视图。
- 会话未过期但租户成员关系发生变更时，页面需及时反映新的可操作边界。
- 跨标签页中一个标签执行租户切换，其他标签页应避免出现静默错位。
- 新租户尚无成员数据时，空态文案需区分“无数据”与“无权限”。
- root 默认不应被前端 `isCurrentTenantAdmin` 判定为当前业务租户 admin。
- 插件物理包已安装但当前租户未启用时，菜单和代理入口都必须拒绝访问。
- 插件全局进程正在运行但当前租户停用时，停用操作不得停止全局进程。
- 插件进程启动环境不得绑定某一个业务租户作为全局运行时身份。
- 插件进入 drain 后，只能阻断目标 `plugin_id` 或 `plugin_id + version` 的新增使用，不得影响其他插件或同租户其他业务。
- 插件实例处于 idle 但入口仍开放时，不得被当作 drained。
- 插件 emergency disable 只能立即禁止目标插件继续被使用，不得删除租户实例、订阅、配置或业务数据。
- SaaS signup 任一步失败时不得留下半成品 tenant/member/role binding。
- 登录凭证属于全局 user；登录入口不得要求用户先选择组织，租户上下文由 request tenant、最近租户或 active member 推导。
- `iam_user.last_tenant_uuid` 只是最近使用偏好，不代表权限；每次使用前必须重新校验当前 user 是否拥有目标租户 active member。
- 手机号注册用户不得被写入虚假的默认邮箱；界面展示应优先使用真实 email，否则使用 phone。
- 历史租户缺少 owner/admin 时必须显式报告，不允许静默降级为 root 代管。
- 注册准入策略缺失、未知模式、未知规则类型、时间窗口不匹配或配额耗尽时，必须明确拒绝，不得自动回退到开放注册。
- 邀请码不得明文落库；失败注册不得消耗邀请码。
- 候补名单和人工审核模式不得提前创建 active 租户。
- 百分比灰度不得使用每次请求随机值；同一 contact 在同一策略版本下必须得到稳定结果。
- 验证码发送必须同样执行注册准入策略，不能在注册关闭或邀请制未满足时泄露可用入口。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 系统必须定义并统一执行三类角色模型：root、租户管理员、普通成员。
- **FR-002**: 系统必须保证 root 可执行跨租户用户管理能力。
- **FR-003**: 系统必须保证租户管理员仅可执行本租户用户管理能力。
- **FR-004**: 系统必须保证普通成员默认不具备租户级用户管理能力。
- **FR-005**: 用户管理页面必须将“查看租户详情”“切换租户上下文”“进入其他页面”定义为独立动作。
- **FR-006**: 点击租户记录不得隐式触发租户切换与页面跳转的复合副作用。
- **FR-007**: 系统必须在进入关键管理页面时以服务端最新身份上下文进行视图分流判定。
- **FR-008**: 当本地缓存与服务端上下文冲突时，系统必须以服务端结果为准并完成状态纠偏。
- **FR-009**: 系统必须提供可识别的上下文异常结果，支持统一会话恢复或重新认证流程。
- **FR-010**: 新租户注册用户必须具备其所属租户的管理员初始能力（除非被明确策略覆盖）。
- **FR-013**: 新租户初始化管理员的角色绑定必须显式包含 `role_admin`（`role_owner` 仅作为可选附加角色），以保证 `me/context.members[].is_admin` 与实际管理权限一致。
- **FR-011**: 跨租户访问尝试必须被拒绝，并返回一致的授权失败语义。
- **FR-012**: 用户管理功能文档必须包含角色边界、交互语义和状态一致性规则，作为实现与验收依据。
- **FR-014**: 系统必须提供 SaaS 自助注册接口，用于事务化创建 tenant、owner user/member、默认角色绑定和基础租户配置。
- **FR-015**: SaaS 自助注册的首个成员必须绑定 `role_owner`、`role_admin`、`role_user`。
- **FR-016**: 已有 user 创建新租户时，系统必须重新校验登录凭证，禁止只凭 email/phone 复用账号。
- **FR-017**: 租户切换必须让 token/context 同步指向新的 `tenant_uuid + member_id/member_uuid`，禁止只修改前端本地状态。
- **FR-017A**: 登录必须以全局 user 凭证为准，支持邮箱或手机号登录；未传租户时必须按 `last_tenant_uuid`、第一个 active member 的顺序选择默认租户。
- **FR-017B**: `last_tenant_uuid` 只能作为登录默认租户偏好，不得绕过 active member 校验；登录和切换成功后必须更新该字段。
- **FR-017C**: SaaS signup 的 `tenant_key` 支持用户显式填写；未填写时由系统根据 `tenant_name` 自动生成唯一 key，显式 key 冲突必须失败并回滚。
- **FR-017D**: SaaS signup 验证码必须由注册准入策略的 `requires_verification` 控制；关闭时前端不展示验证码字段，后端不要求 `verification_code`。
- **FR-017E**: 系统必须提供权威租户注册准入策略，支持 `closed`、`open`、`invite_only`、`waitlist`、`approval_required`、`allowlist`、`progressive_rollout` 模式。
- **FR-017F**: setup 安装流程和 root 后台必须写入同一份注册准入策略对象，公开注册接口不得直接读取旧 `enable_saas_signup` 布尔开关作为运行时兜底。
- **FR-017G**: 公开注册页必须通过 effective policy 查询当前注册模式、是否需要验证码、是否需要邀请码、是否进入候补或审核，不得把前端隐藏控件作为准入边界。
- **FR-017H**: `POST /api/v1/public/saas/signup/verification-code` 与 `POST /api/v1/public/saas/signup` 都必须先执行注册准入策略判定。
- **FR-017I**: 邀请制注册必须校验结构化 `invite_code`，且邀请码消耗必须与租户创建在同一事务中完成；失败时不得消耗邀请码。
- **FR-017J**: 候补名单模式必须只创建注册申请记录，不得创建 tenant/user/member。
- **FR-017K**: 人工审核模式必须在 root 审核通过后才执行租户创建事务；拒绝时保留申请与拒绝原因。
- **FR-017L**: 白名单与灰度规则必须结构化保存；系统不得从自由文本字段解析邀请码、渠道或灰度标识。
- **FR-017M**: 灰度百分比规则必须使用稳定 seed，例如 contact hash；同一 contact 在同一策略版本下的准入结果必须稳定。
- **FR-017N**: 注册准入审计必须记录 `policy_uuid`、`policy_version`、`mode`、`decision`、`reason_code`、`matched_rules`、可选 `invite_code_uuid`、`registration_request_uuid` 和创建成功后的 `tenant_uuid`。
- **FR-017O**: 缺少 active 策略、未知 mode、未知规则 type、配额耗尽、时间窗口不匹配时必须 fail fast 并返回机器可读错误码，不得自动回退到开放注册。
- **FR-018**: root 默认必须进入 Platform Console，且不得被自动视为当前业务租户 owner/admin。
- **FR-019**: root 访问业务租户上下文必须通过 Support Session，并记录 target tenant、reason、actor、start/end time 和写操作审计。
- **FR-020**: 系统必须区分全局 Plugin Package 与 Tenant Plugin Instance。
- **FR-021**: 插件菜单、`/_p/<plugin>/admin`、`/_p/<plugin>/api` 必须校验当前租户是否启用对应插件实例。
- **FR-022**: 租户启用/停用插件不得删除或重装全局插件物理包，也不得影响其他租户实例。
- **FR-022A**: 角色权限目录必须消费 Capability Registry/IAM Permission 中 `source=plugin` 的细颗粒度权限，并按插件、模块、菜单、页面、动作分组展示；用户可见标题和说明必须来自 i18n 元数据。
- **FR-022B**: 角色授权必须绑定插件声明的 `permission_code`，不得绑定 URL、数字 ID、临时 action key 或旧粗权限 alias；API binding 只用于 Gateway 与插件后端校验。
- **FR-022C**: 插件权限登记失败、缺 i18n、缺 `permission_code` 或缺 binding 元数据时，角色权限页必须显示登记失败状态，不允许管理员授权半登记权限。
- **FR-027**: 插件运行时进程必须按全局插件包维度管理，同一 PowerX 节点内同一 `plugin_id` 不得因多个租户启用而启动多组进程。
- **FR-028**: 租户插件实例启用/停用只能改变租户级可见性、配置、凭证和 capability 状态，不得直接停止全局插件进程。
- **FR-029**: 插件后端必须从每次请求或事件上下文解析 `tenant_uuid + member_uuid`，禁止把进程启动时注入的某个租户作为全局当前租户。
- **FR-030**: 插件后台任务事件必须携带 `tenant_uuid`、`plugin_id`、业务 payload 和幂等 key，确保共享进程内的租户隔离。
- **FR-031**: 系统必须明确区分租户生命周期、插件包生命周期、租户插件实例生命周期和插件共享运行时生命周期。
- **FR-032**: 租户暂停或归档时，系统必须关闭该租户插件业务入口和后台任务，但不得停止全局插件运行时。
- **FR-033**: 插件包卸载或全局停用必须由 Root/Platform 执行，并必须检查或处理受影响的租户实例。
- **FR-034**: 租户插件实例过期或停用时，系统必须保留租户配置和历史业务数据，除非执行明确的数据删除流程。
- **FR-035**: 当目标插件仍存在任意租户插件实例时，普通 uninstall 必须拒绝同步卸载并返回 drain required 语义，禁止隐式删除租户实例或强删插件目录。
- **FR-036**: Root 发起插件 drain、uninstall、emergency disable、replace 时，作用范围必须精确限定为目标 `plugin_id`、`plugin_id + version`、`plugin_id + tenant_uuid` 或 `plugin_id + version + tenant_uuid`。
- **FR-037**: 插件 drain 启动后，系统必须禁止目标插件新增订阅、启用、业务写入、scheduler job、queue task、workflow run、webhook/event delivery。
- **FR-038**: 每个租户插件实例必须独立完成 drain 判定；只有活跃 session、写入请求、queue/task/workflow/scheduler、webhook/event 补偿和插件 DrainStatus 均清零或 ready 后，才能进入 drained。
- **FR-039**: emergency disable 必须立即阻断目标插件继续使用并保留租户实例、订阅、配置、凭证引用和历史业务数据。
- **FR-039A**: 插件 final uninstall 必须是目标插件级生命周期操作，只能停止目标插件运行时、卸载目标插件动态路由、更新目标插件 registry，并按 `purge` 清理目标版本物理目录；不得重启 PowerX backend、web-admin、数据库、Redis、Event Fabric、Scheduler、STS 或 Gateway。
- **FR-039B**: 插件 final uninstall 不得影响其他插件的运行时、动态路由、菜单、租户实例、订阅、配置、凭证和业务数据；前端 loading 只能表示卸载请求等待中，不得表达为 PowerX 底座重启。
- **FR-040**: replace installed version 只能替换目标同版本物理包和运行时，不得删除 Tenant Plugin Instance、订阅、权限、配置或业务数据。
- **FR-041**: 生产插件升级必须采用版本化安装、healthcheck、current version 切换和失败回滚语义；不得把同版本 replace 作为常规生产升级路径。
- **FR-023**: IAM 迁移必须保留现有 root user、`system` tenant member、setup 完成记录、组织架构和角色绑定数据。
- **FR-024**: IAM 迁移必须提供只读巡检能力，报告 root、system tenant、业务租户 owner/admin 缺失情况。
- **FR-025**: 对缺少 `role_owner` 但存在 active `role_admin` 的历史租户，系统可以自动补齐 owner，并必须写审计。
- **FR-026**: 对缺少 active admin 的历史租户，系统必须只报告异常，禁止自动猜测 owner。
- **FR-042**: 系统菜单可见性必须接入 IAM RBAC，菜单权限用 `module=menu, resource=<menu_key>, action=read` 表达。
- **FR-043**: 菜单权限必须授予角色，不直接授予单个用户；用户通过当前租户 member 绑定角色获得菜单可见性。
- **FR-044**: `/api/v1/admin/menus` 必须按当前 `tenant_uuid + member_id` 执行角色权限过滤；未授权菜单不得返回给前端。
- **FR-045**: 菜单隐藏不得替代页面/API 权限校验；直接访问 URL 或调用 API 时仍必须由目标模块执行授权。
- **FR-046**: 系统必须内置供应商角色 `role_vendor`，作为租户级 builtin role，可通过 seed/upsert 写入每个租户。
- **FR-047**: `role_user`、`role_readonly`、`role_vendor` 的菜单权限必须显式白名单授权，禁止因为 `action=read` 自动继承全部菜单。
- **FR-048**: 插件/App 菜单权限必须从插件 manifest 的 `frontend.admin.menus` 自动同步生成，禁止管理员手工创建插件菜单资源。
- **FR-049**: 插件/App 菜单权限必须统一使用 `module=menu, resource=plugin.<plugin_id>.<menu_id>, action=read`；插件菜单聚合返回的每个插件菜单项必须自动附加对应权限策略。
- **FR-050**: 插件菜单权限只控制菜单可见性；插件能力/API、`/_p/<plugin>/admin` 和 `/_p/<plugin>/api` 仍必须按插件实例、页面和接口各自的授权规则独立校验。

### Key Entities *(include if feature involves data)*

- **Identity Context**: 当前会话对应的身份上下文，包含用户身份、当前租户、成员关系与角色判定信息。
- **Tenant Membership**: 用户与租户之间的成员关系，定义用户在租户内的角色与管理权限。
- **Role Capability Boundary**: 角色对应的可见范围与可执行动作集合，用于页面分流与操作授权判定。
- **User Management Action**: 用户管理页面内可触发的动作语义实体，至少包含查看详情、切换上下文、页面跳转三类。
- **SaaS Signup Request**: 公开注册新租户的请求，包含 tenant name、可选 tenant key、owner identifier、凭证和初始套餐。
- **Registration Policy**: 租户注册准入策略，定义注册模式、验证码要求、邀请码要求、审核要求、时间窗口、配额和灰度规则。
- **Registration Invite Batch**: 邀请码批次，定义邀请码生成范围、可用次数、有效期、允许套餐和允许渠道。
- **Registration Invite Code**: 单个邀请码，使用 hash 存储，参与注册准入校验和事务化消耗。
- **Registration Request**: 候补名单或人工审核申请，记录申请资料、审核状态、策略版本和转换后的租户 UUID。
- **Registration Policy Audit Event**: 注册准入与策略变更审计，记录每次 allow/deny/pending 决策和命中规则。
- **Root Support Session**: root 进入某业务租户上下文的显式支持会话，必须具备原因和审计边界。
- **Tenant Plugin Instance**: 某租户启用某全局插件包后的租户实例状态和配置。
- **Plugin Drain Job**: root 发起的插件下架或卸载前 drain 计划，负责按目标插件/版本逐租户关闭新增入口、等待存量任务清零并驱动 final uninstall。
- **IAM Migration Report**: 历史 IAM 数据巡检结果，包含可自动补齐项和必须人工处理项。
- **Menu Permission**: 后台菜单入口可见性权限，固定使用 `module=menu/resource/action=read`，由角色授权控制。系统菜单由 seed 创建，插件/App 菜单由插件 manifest 同步创建。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 在角色回归测试中，root、租户管理员、普通成员三类账号的权限判定准确率达到 100%。
- **SC-002**: 在用户管理页交互回归中，“点击租户记录导致非预期跳转”的问题发生率降为 0%。
- **SC-003**: 在缓存冲突与跨标签场景测试中，角色视图与服务端上下文一致率达到 100%。
- **SC-004**: 新租户注册后，租户管理员在 5 分钟内可完成首次成员管理操作的成功率达到 95% 以上。
- **SC-005**: 与用户管理权限语义相关的误报/工单数量在一个发布周期内下降 60% 以上。
- **SC-006**: SaaS signup 在重复 key、错误密码、初始化失败场景中半成品 tenant/member 残留率为 0%。
- **SC-007**: root 默认菜单中租户 AI Settings 和租户插件业务入口误展示率为 0%。
- **SC-008**: 插件租户隔离回归中，未启用租户直接访问插件 admin/api 的拒绝率为 100%。
- **SC-009**: IAM 历史数据巡检报告覆盖 root、system tenant、owner/admin 缺失三类问题，漏报率为 0%。
- **SC-010**: 插件 uninstall 回归中，存在租户实例时同步卸载拒绝率为 100%，且响应可定位到 drain required。
- **SC-011**: 插件 replace 回归中，目标版本替换后租户实例、订阅、配置和业务数据保留率为 100%。
- **SC-012**: 注册准入策略回归中，七种注册模式的后端判定准确率达到 100%，且验证码发送与 signup 判定一致。
- **SC-013**: 邀请码注册在成功、失败、重复提交和事务回滚场景下的邀请码错误消耗率为 0%。
- **SC-014**: 灰度百分比规则在同一 contact、同一策略版本下的准入结果稳定率为 100%。
- **SC-015**: root 一键切换到 `closed` 后，公开验证码发送和 signup 新增租户阻断率为 100%。

## Assumptions

- 用户管理模块属于高敏感管理域，默认采用“最小权限”原则。
- root 账号为平台级特殊身份，不受单租户成员可见性限制。
- 租户管理员初始能力由租户注册流程或管理员授权流程保证。
- 本规格不扩展外部身份源同步细节，仅约束平台内身份与交互语义一致性。
- 现有 root 初始化记录和 `system` tenant member 需要保留，不能通过手动删库方式切换 SaaS 语义。
- 插件物理包仍是全局安装，SaaS 隔离发生在租户实例配置和代理访问控制层。

## Dependencies

- 统一身份上下文查询能力可稳定返回当前角色与租户信息。
- 现有 RBAC 角色判定与授权链路可承载三类角色边界。
- 用户管理前端页面具备按身份分流与动作拆分的承载能力。
