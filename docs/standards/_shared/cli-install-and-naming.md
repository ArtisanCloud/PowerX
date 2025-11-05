
# PowerX CLI 家族 · 统一安装与规范

> 目标：PowerX 各仓（PX / PX-ADMIN / PLG / MKP）CLI 命名统一、安装统一、版本统一。
> 语言：Golang。安装源：GitHub。适用系统：macOS / Linux / Windows。

## 1. 命名与仓库映射

| 角色                      | 二进制名（推荐）    | Go 包路径（示例）                                             | 仓库                            |
| ----------------------- | ----------- | ------------------------------------------------------ | ----------------------------- |
| PowerX（Backend，PX）      | `px`        | `github.com/ArtisanCloud/PowerX/cmd/px`                | `ArtisanCloud/PowerX`         |
| PowerX Web Admin（PX-ADMIN） | `px-admin`  | `github.com/ArtisanCloud/PowerX/cmd/px-admin`         | `ArtisanCloud/PowerX`         |
| PowerX Plugin（PLG）      | `px-plugin` | `github.com/ArtisanCloud/PowerXPlugin/cmd/px-plugin`   | `ArtisanCloud/PowerXPlugin`   |
| PowerX Marketplace（MKP） | `px-market` | `github.com/Matrix-X/PowerXPluginMarket/cmd/px-market` | `Matrix-X/PowerXPluginMarket` |

> 约定：
>
> * 二进制命名统一为 `px-<scope>`（PX 主入口简写为 `px`）。
> * 各仓 CLI 的 `main.go` 放在 `cmd/<binary>/` 目录，便于 `go install`。

## 2. 快速安装（Go ≥ 1.22）

> 如果网络直连 GitHub 略慢，先配置 Go 代理（可选）：
> `export GOPROXY=https://proxy.golang.org,direct`

**安装最新版本：**

```bash
# PX
go install github.com/ArtisanCloud/PowerX/cmd/px@latest
# PX-ADMIN
go install github.com/ArtisanCloud/PowerX/cmd/px-admin@latest
# PLG
go install github.com/ArtisanCloud/PowerXPlugin/cmd/px-plugin@latest
# MKP
go install github.com/Matrix-X/PowerXPluginMarket/cmd/px-market@latest
```

**安装指定版本（建议在 CI 固定版本）：**

```bash
go install github.com/ArtisanCloud/PowerX/cmd/px@v1.12.0
go install github.com/ArtisanCloud/PowerX/cmd/px-admin@v1.12.0
go install github.com/ArtisanCloud/PowerXPlugin/cmd/px-plugin@v1.12.0
go install github.com/Matrix-X/PowerXPluginMarket/cmd/px-market@v1.12.0
```

> 安装路径：`$GOPATH/bin`（若未设 GOPATH，则为 `$HOME/go/bin`）。
> 请确保该目录已加入 `PATH`。

**验证：**

```bash
px --version
px-admin --version
px-plugin --version
px-market --version
```

## 3. 通过 GitHub Releases 安装（可选）

> 若各仓提供预编译二进制（建议命名）：
> `px-<scope>_<version>_<os>_<arch>.tar.gz`
> 示例：`px-plugin_v1.12.0_darwin_amd64.tar.gz`

**通用安装脚本（bash，macOS/Linux）：**

```bash
# 用法：install_px_bin <owner/repo> <binary> <version> <os> <arch>
install_px_bin() {
  REPO="$1"; BIN="$2"; VER="$3"; OS="$4"; ARCH="$5"
  URL="https://github.com/${REPO}/releases/download/${VER}/${BIN}_${VER}_${OS}_${ARCH}.tar.gz"
  TMP="$(mktemp -d)"
  curl -fsSL "$URL" -o "$TMP/${BIN}.tar.gz"
  tar -xzf "$TMP/${BIN}.tar.gz" -C "$TMP"
  install "$TMP/$BIN" /usr/local/bin/$BIN
  rm -rf "$TMP"
  command -v $BIN >/dev/null && $BIN --version
}

# 示例：
install_px_bin ArtisanCloud/PowerX px v1.12.0 $(uname -s | tr '[:upper:]' '[:lower:]') amd64
install_px_bin ArtisanCloud/PowerX px-admin v1.12.0 $(uname -s | tr '[:upper:]' '[:lower:]') amd64
install_px_bin ArtisanCloud/PowerXPlugin px-plugin v1.12.0 $(uname -s | tr '[:upper:]' '[:lower:]') amd64
install_px_bin Matrix-X/PowerXPluginMarket px-market v1.12.0 $(uname -s | tr '[:upper:]' '[:lower:]') amd64
```

> Windows（PowerShell）可直接下载对应 zip 并手动放入 `PATH` 目录，或用 Scoop/Chocolatey 打包分发（可选）。

## 4. 升级 / 卸载

**升级到最新：**

```bash
go install github.com/ArtisanCloud/PowerX/cmd/px@latest
go install github.com/ArtisanCloud/PowerX/cmd/px-admin@latest
go install github.com/ArtisanCloud/PowerXPlugin/cmd/px-plugin@latest
go install github.com/Matrix-X/PowerXPluginMarket/cmd/px-market@latest
```

**卸载：**

```bash
rm -f "$(go env GOPATH)/bin/px" \
      "$(go env GOPATH)/bin/px-admin" \
      "$(go env GOPATH)/bin/px-plugin" \
      "$(go env GOPATH)/bin/px-market"
```

## 5. Shell 补全（如已实现）

```bash
# bash
px completion bash    | sudo tee /etc/bash_completion.d/px >/dev/null
px-admin completion bash | sudo tee /etc/bash_completion.d/px-admin >/dev/null
px-plugin completion bash | sudo tee /etc/bash_completion.d/px-plugin >/dev/null
px-market completion bash | sudo tee /etc/bash_completion.d/px-market >/dev/null

# zsh
px completion zsh          > "${fpath[1]}/_px"
px-admin completion zsh    > "${fpath[1]}/_px-admin"
px-plugin completion zsh   > "${fpath[1]}/_px-plugin"
px-market completion zsh   > "${fpath[1]}/_px-market"
```

> 需要在各 CLI 中暴露 `completion` 子命令（Cobra 自动支持）。

## 6. 版本与输出规范（强烈建议）

* 统一打印版本格式：

  ```
  PowerX CLI Family v1.12.0 (binary: px, commit: abcdef0, date: 2025-10-20)
  ```
* 均支持：

  ```
  --version / version
  --help    / help
  ```
* 支持 `PX_CI=1`（或 `--ci`）输出纯机器可读日志，便于 CI 集成。

## 7. FAQ

* **提示 `command not found`？**
  把 `$(go env GOPATH)/bin` 或 `$HOME/go/bin` 加入 `PATH`。

* **公司网络访问 GitHub 慢？**
  使用 `GOPROXY` 或者改用 Releases 预编译二进制。

* **多版本并存？**
  用版本号后缀安装到不同路径，或使用 `asdf`/`rtx` 自定义 shim。

---

## 8. Pure Push 分发流程（PowerXDocs ➜ 下游仓）

PowerXDocs 提供以下自动化脚本，确保所有 CLI 与文档以 “纯 Push” 同步到各仓：

| Workflow | 命令 | 说明 |
|----------|------|------|
| Usecase 模板分发 | `npm run publish:usecases -- --scn-id SCN-XXXX` | 读取 `docs/_data/docmap.yaml` 与 `docs/_data/repos.yaml`，将更新后的模板推送到 `_from_hub/` 目录并生成报告。 |
| Standards 分发 | `npm run publish:standards` | 将 `docs/standards/**` 拷贝到各仓对应的 standards 目录，保持治理文案一致。 |
| 审核提醒 | `npm run publish:notify -- --workflow usecases` | 检查超过 72 小时未合并的 PR，输出提醒信息。 |

### 8.1 工作流特性

- **幂等/可重跑**：每次执行都会生成 `reports/_state/**` 的运行指纹；若内容未变更，脚本会拒绝重复分发并要求使用上一次报告中的 `resumeToken` 重试。
- **报告输出**：所有工作流在 `reports/usecases/` 与 `reports/standards/` 下生成 JSON，记录分发仓库、变更文件、PR 链接、状态与重试 token。
- **Dry Run 支持**：通过 `--dry-run` 可仅复制文件、生成报告，不执行 git commit/push，便于预览。
- **Read-only 约束**：Standards 分发脚本会强制拷贝到 `docs/standards/`，不允许写入其他目录。

### 8.2 操作步骤

1. 确保四个下游仓库已在本地 `repos/<repo-key>` 目录完成 clone，并保持干净工作区。
2. 根据需要更新 `docs/usecases-seeds/**` 或 `docs/standards/**`。
3. 运行对应脚本（建议搭配 `--dry-run` 预检查），确认报告无错误后再去除 `--dry-run`。
4. 脚本会生成带前缀 `docs/hub/...` 的分发分支，创建 PR 并记录在报告中。
5. 使用 `npm run publish:notify` 检查超时 PR，确保遵循 72 小时提醒策略。

> 提示：`docs/_data/repos.yaml` 定义了每个仓的 `checkout` 目录、`usecase_seed_root`、`standards_root` 等元信息，如需调整目录结构请先更新该表。
