# US1：插件会话恢复、续期与切换验收

> 状态：修复实施前的验收计划；以[总计划](guide.md)的版本、分工与风险控制为准。

## 1. 场景与前置条件

- 验收对象：PowerX 宿主、Framework、AI Craft；CRM 等插件按审计结果追加。
- 环境：先本地恢复库，再开发环境。浏览器正常登录，真实认证开启。
- 记录三个项目 commit、Framework 包版本、插件安装版本／实际 JS SHA256、宿主 origin、iframe origin、Core 与插件数据源。
- 本地恢复库当前租户为 `1edd4132-1644-412d-abb4-d5f1e9487052`；该 UUID 仅是本次验收数据，不能硬编码为产品默认租户。
- 初始登录场景使用干净的测试浏览器配置。复现旧标签页场景时明确保留一个旧会话页面，不在未知标签页中批量删除存储。

## 2. 页面与接口操作

| 步骤 | 动作与入口 | 预期结果 | 失败处理 |
| --- | --- | --- | --- |
| 1 | 在 `/users/login` 用现有有效账号登录 | login 200，随后 context 200，当前租户与该环境数据库一致 | 记录响应错误码／trace；不修改账号或租户 |
| 2 | 打开 `/_p/com.powerx.plugins.ai-craft/admin/operations/ai-craft/catalog/process-flows` | 插件进入 ready，业务接口按当前身份请求 | 同时查看 iframe Document 与 context；不要用“表格有数据”替代认证检查 |
| 3 | 插件页面保持打开，在宿主用户菜单退出并重新登录 | 旧插件内存被清理／替换，新会话持续有效 | 对齐登录时间、storage 变化及 context 的租户／trace |
| 4 | 再打开一个旧会话插件标签页，执行登录／切租户 | 旧标签页不能覆盖宿主新会话 | 确认真实 storage 事件与插件内存状态，不只手工调用 setAuth |
| 5 | 经测试夹具让刷新／context 响应延迟，再登录或退出 | 旧成功与旧失败都不提交新会话；退出不复活 | 检查代理 Set-Cookie、内存、存储三处；禁止只测试 JSON 回包 |

只读 context 验证可在宿主浏览器 Console 执行，输出不包含 Token：

```javascript
(async () => {
  const token = localStorage.getItem('access_token');
  if (!token) { console.log({ authenticated: false }); return; }
  const response = await fetch('/api/v1/admin/user/auth/me/context', {
    headers: { Authorization: `Bearer ${token}` }, cache: 'no-store'
  });
  const body = await response.json();
  console.log({
    status: response.status,
    tenant_uuid: body.data?.current_tenant_uuid ?? null,
    trace_id: response.headers.get('x-trace-id'),
    request_id: body.request_id,
    message: body.message
  });
})()
```

这是对当前凭证的一次读取检查，不会修复存储，也不能证明多页面、续期或退出行为已正常。

## 3. 回归矩阵

| 编号 | 输入／操作 | 成功标准 |
| --- | --- | --- |
| S01 | 宿主登录后首次进入插件 | 一次初始化进入 ready，用户／成员／租户一致 |
| S02 | 刷新宿主插件路由 | 会话不被旧 Cookie、alias 或内存值覆盖 |
| S03 | 插件已打开时退出后登录 | 新会话保持；已退出代际不能恢复 |
| S04 | 两个同源插件页面＋旧标签页 | 真实 storage 通知不触发旧 Token 写回 |
| S05 | 宿主与 iframe 不同 origin | 通过受校验桥接同步，不依赖共享 localStorage |
| S06 | 切租户，旧 context 在途 | UI 和请求使用新租户；旧 context 不覆盖当前权限／菜单 |
| S07 | access 到期，多个 API／守卫／WS 同时请求 | 刷新按会话去重；新 access／refresh 快照一致 |
| S08 | 旧 refresh 成功在新登录后返回 | 不覆盖新存储、内存或 Cookie |
| S09 | 旧 refresh／context 失败在新登录后返回 | 不清除新会话，不重定向新页面 |
| S10 | storage 缺失、宿主会话仍有效 | 重新向宿主请求当前会话；相同 fingerprint 也能恢复本地消费状态 |
| S11 | 宿主明确退出，旧插件仍活动 | 插件停止业务请求、WS 断开；旧缓存不复活 |
| S12 | 当前用户业务权限拒绝 403 | 显示权限错误；不触发 refresh 或整页重载循环 |
| S13 | 伪造消息来源／错误 plugin／旧协议 | 明确拒绝，当前会话不变 |
| S14 | iframe Document、图片、下载／资源请求 | 仍按既有代理鉴权访问；续期后不使用过期 Cookie |
| S15 | WS 在首次加载、续期、切租户和退出时 | 建连／重连使用当前快照；旧连接释放，不跨租户订阅 |
| S16 | standalone 插件登录、续期、退出 | 独立会话按原合同正常；不能因嵌入模式修复失效 |
| S17 | 插件局部鉴权错误 | 插件不能随意清空宿主新会话或其他插件的业务偏好 |

同源多页面 E2E 为必须项；仅 jsdom 手工派发事件不能独立作为最终通过依据。

## 4. 测试与验收边界

1. 单元测试验证共享状态机、代际比较、退出标记和快照提交。
2. 集成测试验证 Framework adapter、context 单飞与跨会话取消、代理 Cookie 的响应行为。
3. 真实浏览器验证 S01–S17；保留带鉴权的 Document、API 与 WS 时间线。
4. 不使用默认 `npm run test:e2e` 的 skip-auth 环境作为认证证明。实际 E2E 配置需先提交、确认其鉴权开启，再运行。
5. 记录每项结果为通过／失败／未执行，附当前会话身份元数据与真实 trace。HAR 常包含 Authorization／Cookie／刷新响应，先脱敏再共享。

## 5. 失败处理与回滚

- 不一致时冻结该会话的恢复写入，展示明确错误；保留 trace 和版本组合。
- 桥接超时最多进行合同规定的有限恢复，不能新增轮询或无限自动重载兜底。
- 回滚 Core／Framework／插件的配套版本，关闭旧测试标签页后重测；不回滚或清空业务数据库。
- 认证通过仍不等于 AI Craft 已接入恢复库。另核对插件连接及租户映射后，才能验收业务记录展示。
