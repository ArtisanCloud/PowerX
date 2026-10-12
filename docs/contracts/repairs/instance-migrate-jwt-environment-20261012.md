# 正式 / 开发实例迁移 JWT 配置修复（2026-10-12）

## 原因与已执行远程修复

远程 `sudo ./database migrate -config ...` 在配置校验阶段停止，数据库迁移尚未开始。正式和开发 YAML 的 `auth.jwt_secret` 都保留了旧公开默认值；两套 systemd 服务分别通过自己的 `powerx.env` 读取有效 64 字符 `POWERX_AUTH_JWT_SECRET`。

已只读核对两个运行进程的环境：各自与对应 env 文件密钥一致，正式、开发密钥彼此不同。随后对 `/etc/powerx/config.yaml` 和 `/etc/powerx-dev/config.yaml` 仅同步 `auth.jwt_secret` 为该实例正在使用的原密钥。YAML 其他字段和注释、文件 owner/group/mode 均保留，未改变 env 文件。没有生成新密钥、替换运行二进制或重启服务。

旧配置保存于服务器私有备份目录（目录 0700，文件 0600）：

```
/home/ubuntu/.local/state/powerx-config-repairs/20261012T015834Z/powerx-backend.yaml
/home/ubuntu/.local/state/powerx-config-repairs/20261012T015834Z/powerx-dev-backend.yaml
```

现有 `/opt/powerx/backend/database` 在清除继承的 POWERX/CORE_X 环境后，分别执行 `status -config /etc/powerx/config.yaml` 和 `status -config /etc/powerx-dev/config.yaml` 均退出 0，配置校验与数据库连接通过。此验证是只读状态查询，不是已执行两套迁移的证据。

使用者现在可以在 `/opt/powerx/backend` 直接执行原命令：

```bash
sudo ./database migrate -config /etc/powerx/config.yaml
sudo ./database migrate -config /etc/powerx-dev/config.yaml
```

## Core 长期修复

普通 `config.Load` / `InitGlobalConfig` 从显式配置目录读取 `powerx.env`；已传入进程环境优先，实例文件其次，YAML 再次。实例文件存在时不搜索工作目录的开发 `.env`。作用域环境仅覆盖当前配置对象，不写入全局进程环境，确保同进程顺序或并发加载两个实例不会串用密钥 / 数据库。

Setup 使用相同实例文件签名密钥来源，保留有效外部密钥，不把已有安装的密钥重新生成。公开默认值、占位值、短密钥和旧 JWT 变量名仍拒绝。实例文件损坏 / 不可读时明确失败，错误不输出密钥。文件支持单行 KEY=VALUE、单/双引号，不执行 shell 或变量展开。

`LoadForUpgrade` 保持原先的显式进程环境与严格安装 / 权限 / 数据挂载合同，不搜索开发 `.env`；升级执行器应继续提供其固定的运行环境。普通迁移加载增强不改变正式升级协议。

## 验证与部署

- `go test -race ./config ./cmd/database` 通过，包括两实例顺序 / 并发隔离、进程覆盖优先级、密码字面值、配置不写回、损坏环境文件不泄露值、JWT 拒绝与 Setup 不轮换。
- 对远程实际 YAML 与对应 env 密钥一致性、CLI 状态查询做了真实验证，凭证未输出。
- 当前远程 YAML 修复已生效，不需更新二进制才可重试原命令。自动实例环境加载代码需随正常发布更新二进制后生效，不要求额外数据库迁移。
