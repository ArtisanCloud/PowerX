# Docker 镜像存储策略（独立讨论稿）

## 1. 背景

PowerX 采用 `docker compose` 进行部署时，镜像存储位置会直接影响：

- 发布速度与稳定性
- 权限与安全治理
- 回滚效率
- 离线/内网场景可用性

本文件用于沉淀镜像存储选型与执行规范，作为后续部署与运维评审基线。

## 2. 可选存储位置

### 2.1 私有镜像仓库（主推荐）

类型：

- 云厂商托管：ECR / ACR / GAR / 阿里云 ACR / 腾讯 TCR
- 自建仓库：Harbor / Docker Registry v2

优点：

- 适配标准 `docker pull` / `docker compose`
- 便于权限控制（命名空间、机器人账号、只读 token）
- 支持镜像漏洞扫描与生命周期策略
- 天然支持多环境发布（dev/staging/prod）

风险点：

- 需要维护仓库访问凭证
- 自建仓库需要额外运维（证书、备份、容量）

### 2.2 CI 平台 Registry（次推荐）

类型：

- GHCR（GitHub Container Registry）
- GitLab Container Registry

优点：

- 与 CI/CD 流程天然集成
- 版本与代码提交关联清晰

风险点：

- 组织权限模型复杂时，需要明确团队级访问边界
- 某些企业网络对外访问限制较多

### 2.3 离线镜像包（补充方案）

方式：

- `docker save` 导出 `tar`
- 存放到 MinIO/S3/Nexus/Artifactory
- 目标机 `docker load` 导入

优点：

- 适配强隔离内网与等保场景
- 可作为灾备手段

风险点：

- 包管理与分发流程需要额外规范
- 容易出现“版本包与运行版本不一致”

### 2.4 部署机本地缓存（不建议作为正式方案）

优点：

- 上手简单

风险点：

- 不可审计、不可协作、不可追溯
- 不利于多节点一致性与快速回滚

## 3. PowerX 推荐策略

### 3.1 主路径

- 正式环境使用**私有镜像仓库**（云托管或 Harbor）
- Compose 文件仅引用仓库镜像，不依赖本地 build

示例：

```yaml
services:
  powerx-backend:
    image: registry.example.com/powerx/backend:${POWERX_VERSION}
  powerx-web-admin:
    image: registry.example.com/powerx/web-admin:${POWERX_VERSION}
```

### 3.2 兜底路径

- 为每个发布版本生成离线 `tar` 包并归档
- 归档保留最近 N 个稳定版本（建议 N>=3）

### 3.3 Tag 与不可变约束

- 同时使用：
  - 语义版本 tag（如 `v1.8.2`）
  - 提交 hash tag（如 `sha-abc1234`）
- 生产发布以 hash 为准，语义版本用于业务可读性
- 禁止覆盖已发布 tag（tag immutability）

## 4. 多环境命名规范（建议）

```text
registry.example.com/powerx/backend:prod-v1.8.2
registry.example.com/powerx/backend:staging-v1.8.2
registry.example.com/powerx/web-admin:prod-v1.8.2
```

或统一使用：

```text
registry.example.com/powerx/backend:v1.8.2
registry.example.com/powerx/backend:sha-abc1234
```

并通过不同环境变量 `POWERX_VERSION` 控制部署版本。

## 5. 安全与治理要求

- 仓库访问凭证使用最小权限（Pull-only for runtime）
- 定期轮换机器人账号 token
- 开启镜像漏洞扫描（高危漏洞阻断进入 prod）
- 对发布镜像做签名（如 cosign，可后续启用）
- 保留镜像与部署记录映射，满足审计追踪

## 6. 运维执行清单

- [ ] 已选择主仓库（云托管或 Harbor）
- [ ] 已配置 CI 推送权限与运行时拉取权限
- [ ] 已启用镜像保留与清理策略
- [ ] 已定义 tag 不可变策略
- [ ] 已建立离线包归档流程（应急）
- [ ] 已验证回滚版本可用（镜像可拉取）

## 7. 当前结论（PowerX 本项目）

- 生产推荐：**私有镜像仓库 + 离线 tar 应急备份**
- 不建议：仅依赖部署机本地缓存
- 文档状态：可执行基线，后续可扩展签名与 SBOM 治理

