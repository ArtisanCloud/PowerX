# PowerX Web Admin 本地开发环境搭建指南

> 适用读者：PowerX Web Admin 的前端/全栈工程师、QA、技术支持；默认你已拥有 Git 使用经验并可以访问内部依赖服务。

---

## 快速清单

- Node.js ≥ 20.11（推荐使用 `nvm` 或 `fnm` 管理多版本）。
- 包管理器默认使用 `npm`，确保 `npm@10` 以上。
- 克隆仓库后执行 `npm install`，完成 Nuxt 预处理（自动触发 `nuxt prepare`）。
- 根据 `.env.example` 创建 `.env`，至少填入 `UPSTREAM` 与 `WS_UPSTREAM`。
- 启动热更新服务器：`npm run dev`（默认端口 `3000`）。
- 功能开发或文档更新后，必要的验证流程：打开 `http: "//localhost:3000` → 登录/切换语言 → 访问 Agent、插件市场、仪表盘。"

---

## 1. 基础要求

| 组件 | 版本建议 | 备注 |
| --- | --- | --- |
| Node.js | `>= 20.11.0` | Nuxt 4 依赖 Node 20 新特性，低版本将触发 ESM 解析错误。 |
| npm | `>= 10.2.0` | 旧版本在安装 `@nuxt/ui` 时可能出现 peer 依赖解析失败。 |
| Git | 任意支持 LFS | 仓库包含较大的设计资产（若开启）。 |
| 系统 | macOS 14 / Ubuntu 22.04 / Windows 11 WSL2 | 推荐 Unix 环境以获得一致的 shell 行为。 |

### 1.1 检查当前版本

```bash
node -v
npm -v
git --version
```

若 Node 版本低于 20，可安装 `nvm` 后切换：

```bash
curl -fsSL https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
nvm install 20
nvm use 20
```

---

## 2. 克隆仓库

```bash
git clone git@github.com:artisancloud/powerx.git
cd powerx
```

> 如果你使用 HTTPS，可替换为 `https://` 地址。建议在 Mac 上启用 Keychain，减少频繁输入密码。

---

## 3. 安装依赖

首次进入仓库后执行：

```bash
npm install
```

- `postinstall` 会自动运行 `nuxt prepare` 生成类型声明。
- 如果本地全局安装了 `pnpm` 或 `yarn`，仍建议使用 `npm`，保持与 CI 环境一致。
- 安装过程中若遇到网络波动导致的 404，可配置公司内部 npm 镜像或使用 `npm config set registry <mirror-url>`。

---

## 4. 配置环境变量

复制示例文件：

```bash
cp .env.example .env
```

推荐至少确认以下键值：

| 变量 | 作用 | 默认值 | 备注 |
| --- | --- | --- | --- |
| `NUXT_DEFAULT_LANGUAGE` | UI 默认语言 | `zh` | 支持 `zh,en,ja,ko`。 |
| `NUXT_FORCE_THEME` | 强制主题 | `dark` | 若希望跟随系统，可改为 `auto`。 |
| `UPSTREAM` | REST API 网关地址 | `http://127.0.0.1:8077` | 对接后端 Mock/本地接口。 |
| `WS_UPSTREAM` | WebSocket 地址 | `ws://127.0.0.1:3001` | 本地调试实时消息时需保持可达。 |

- 更完整的变量说明参考 `docs/environment/Env_Variables_Schema.md`（补全中）。
- `.env` 默认被列入 `.gitignore`，请勿提交到版本库。如需共享环境配置，可在团队 wiki 或私有 vault 中维护。

---

## 5. 启动开发服务器

```bash
npm run dev
```

- 默认监听 `http://localhost:3000`，可通过 `--port` 修改：`npm run dev -- --port 4000`。
- Nuxt 会热加载 `app/`、`server/`、`i18n/` 等目录的变更。
- 首次启动若生成 `.nuxt/` 缓存较慢，可开启 `NUXT_DEVTOOLS=1` 以启用 DevTools（见 [Nuxt DevTools](https://devtools.nuxt.com/)）。

---

## 6. 验证运行状态

1. 浏览器访问 `http://localhost:3000`，确认欢迎页加载正常。
2. 打开浏览器控制台，确保无 4xx/5xx 与 `WebSocket` 连接错误。
3. 若本地后端未启动，可在 `app/stores` 中关注是否有降级逻辑（部分页面会展示 Mock 空态）。
4. 手动验证关键页面：
   - Agent 工作区（`/agent`）—— 发起会话、查看消息渲染。
   - 插件市场（`/plugins/market`）—— 检查分类与安装按钮。
   - 仪表盘（`/dashboard`）—— 确认图表加载。

---

## 7. 推荐开发流程

```bash
# 1. 切换至新分支
git checkout -b feat/some-feature

# 2. 更新依赖或变更代码
npm install

# 3. 启动开发服务器
npm run dev

# 4. 完成开发后进行必要验证
bash scripts/check-refactor.sh   # 调整 Agent 相关结构时执行

# 5. 构建产物或生成静态包（可选）
npm run build
npm run preview
npm run generate
```

- 如需运行 ESLint：`npx eslint . --ext .ts,.vue --fix`。
- 功能合并前建议至少手动走通认证、仪表盘、Agent 聊天三条主路径。

---

## 8. 常见问题与排查

**Node 版本错误**  
症状：启动时报错 `SyntaxError: Cannot use import statement outside a module`。  
解决：升级 Node 至 ≥20，删除 `node_modules` 与 `package-lock.json` 后重新安装。

**端口冲突**  
症状：`Error: listen EADDRINUSE: address already in use :::3000`。  
解决：使用 `lsof -i :3000` 查找占用进程，或通过 `npm run dev -- --port 4000` 更换端口。

**API 无法访问**  
症状：控制台持续报错 `Failed to fetch`。  
解决：确认后端 Mock/真实服务是否启动，或将 `UPSTREAM` 指向可用地址。可临时启用 MSW（待在 `docs/environment/Local_Mocks_and_Fixtures.md` 中补充）。

**依赖安装失败**  
症状：`npm ERR! code ERESOLVE` 或网络超时。  
解决：执行 `npm cache clean --force`，切换到公司镜像或开启代理；在 CI 指定版本号以避免漂移。

---

## 9. IDE 与工具建议

- VS Code 搭配扩展：`Volar`, `ESLint`, `Tailwind CSS IntelliSense`。
- 配置 VS Code `settings.json` 中的 `"vue.features.codeActions.enabled": true`，以获得 `<script setup>` 自动补全。
- 推荐启用 EditorConfig（仓库根目录已有 `.editorconfig`，若无请与团队确认）。

---

## 10. 进一步阅读

- `docs/environment/Env_Variables_Schema.md` —— 完整环境变量对照表。
- `docs/environment/Local_Mocks_and_Fixtures.md` —— 本地数据模拟策略（待完成）。
- `docs/testing/Manual_Test_Paths.md`（如存在）—— QA 手工回归路径。

如需私有化部署或 CI/CD 说明，请前往 `docs/build-and-deploy/` 目录。
