# PowerX – 企业级 AgentOS 操作系统

[![English](https://img.shields.io/badge/English-README-blue)](./README-en.md)
[![Official Docs](https://img.shields.io/badge/Official_Docs-powerx.artisan--cloud.com-green)](https://powerx.artisan-cloud.com)
[![License](https://img.shields.io/badge/License-Apache%202.0-red.svg)](LICENSE)

> PowerX 是企业级 **Agent Operating System (AgentOS)**，以 **插件化智能体** 为核心的企业操作系统。它通过 AI Agent 编排、插件生态和统一协议，让企业业务模块（CRM、电商、SCRM、审批流等）以智能体形式 **自主协作与进化**。
>
> **PowerX = AI Agent 编排内核 + 插件市场 + MCP/gRPC/REST 多协议统一**

---

## 🌟 预览图

### 管理后台

![管理后台](https://raw.githubusercontent.com/ArtisanCloud/PowerXDoc/dev/michaelhu/docs/website/public/images/px-home-zh.png)

### 插件市场

![插件市场](https://raw.githubusercontent.com/ArtisanCloud/PowerXDoc/dev/michaelhu/docs/website/public/images/px-market-zh.png)

---

## 🏗️ 架构哲学

PowerX 是 **企业级 AgentOS（Agent Operating System）**，遵循三个核心原则：

### 1. 内核最小化

- 内核只提供通用能力：用户与组织（IAM）、权限与访问控制（RBAC）、事件总线（Event Bus）、审计（Audit）、数据库抽象（DB Layer）、运行时流引擎（Flow）
- 这些能力作为 SDK (`pkg/corex/*`) 暴露，任何插件和外部系统都可以复用

### 2. 智能体优先（Agent-First）

- 业务功能（电商、SCRM、审批流、直播…）以 **AI Agent 智能体** 形式存在
- 智能体通过 **契约（Contract）** 与内核对接，独立开发、独立部署、独立数据库 Schema
- 智能体可由官方/第三方/客户自己开发，并通过 **插件市场** 分发，支持自主学习和协同

### 3. 契约驱动 + AI 原生

- 插件之间 **不直接依赖**，通过 **契约接口 / 事件订阅** 解耦
- 支持 **MCP (Model Context Protocol)** 标准化 AI 工具接入
- 所有对外接口（HTTP/gRPC）、事件（Event Topics）、数据模型（OpenAPI/Proto）均在 `/contracts` 目录集中管理
- 内置工具链会自动生成 SDK（Go/TypeScript），保证前后端一致

---

## 🔑 核心组件

| 组件 | 功能描述 | 技术特性 |
|------|----------|----------|
| **IAM（身份与组织）** | 用户、部门、角色、标签 | 所有插件共享的组织架构 |
| **RBAC（权限控制）** | 统一权限模型（Role/Policy） | 可扩展至插件级别：插件只声明"资源与动作"，授权由内核托管 |
| **Event Bus（事件总线）** | 插件间的主要通信机制 | 支持 Local/Redis 实现，插件只需订阅 Topic，不关心消息来源 |
| **Audit（审计日志）** | 统一采集所有事件/操作 | 内核级别可扩展至合规（安全/风控） |
| **DB Layer（数据库抽象）** | 多租户隔离（Tenant） | 插件独立 Schema，但共享 Postgres/MySQL 实例，统一迁移工具（Goose 兼容） |
| **Flow（业务流程引擎）** | 内置可编排的执行流（Plan/Task/Node） | 插件可挂载定制 Flow 节点 |
| **Agent Lifecycle & Observability（代理生命周期与可观测性）** | 覆盖代理注册、激活、暂停、扩缩容与退役的完整控制平面（HTTP / gRPC） | 内建健康评分聚合、趋势查询、订阅过滤与企业 IM 告警，支持 13 个月保留策略 |

---

## 🔌 插件机制

### 插件结构

```
插件包/
├── plugin.yaml          # 插件元数据
├── backend/             # 后端可执行文件
│   └── main
├── web-admin/           # 前端资源
│   ├── pages/
│   └── assets/
└── contract/            # 契约定义
    └── api.yaml
```

### 插件生命周期

- **安装**：将插件包放入 `/plugins` 目录
- **注册**：系统启动时自动扫描并注册插件
- **加载**：动态加载插件菜单和页面
- **通信**：通过事件总线或契约接口与其他插件协作

---

## 🖥️ 多端支持

PowerX 内置四类前端壳，共享 **统一契约**，SDK 自动生成，支持多种协议和框架：

| 前端壳/协议 | 适用场景 | 特性 |
|------------|----------|------|
| **Admin 管理后台** | 运营/管理人员 | 动态加载菜单和插件页面 |
| **Web User 前端** | C 端用户界面 | 响应式设计，支持多端 |
| **MiniApp 小程序** | 轻量级触达场景 | 微信/支付宝小程序支持 |
| **OpenAPI** | 第三方系统 | RESTful API 统一调用接口 |
| **MCP (Model Context Protocol)** | AI Agent 与插件交互 | 标准化 AI 工具接入协议 |
| **gRPC** | 高性能服务间通信 | 基于 Protobuf 的高效 RPC |

---

## 📚 文档资源

详细的安装、部署、使用指南请参考：

- **🚀 [官方文档 - 快速开始](https://powerx.artisan-cloud.com)** - 完整的安装部署指南
- **📖 [API 文档](https://powerx.artisan-cloud.com/api-docs)** - 接口规范与示例
- **🏗️ [架构设计](https://powerx.artisan-cloud.com/architecture)** - 深入理解系统设计
- **🔌 [插件开发指南](https://powerx.artisan-cloud.com/plugin-development)** - 开发自定义插件
- **☁️ [部署指南](https://powerx.artisan-cloud.com/deployment)** - 生产环境部署方案
- **🛠️ [运维手册](https://powerx.artisan-cloud.com/operations)** - 监控、备份、升级
- **📘 [Knowledge Space Quickstart](specs/011-knowledge-space/quickstart.md)** - 端到端创建/入库/融合/反馈示例
- **🧯 [Knowledge Space Runbook](docs/guides/knowledge_space/runbook.md)** - 入库/融合/反馈故障处理与脚本
- **📊 [Perf & Resiliency Validation](docs/guides/knowledge_space/perf_validation.md)** - 压测/降级/反馈风暴验证
- **✅ [Smoke Checklist](docs/guides/knowledge_space/smoke_checklist.md)** - 发布前的冒烟检查表

---

## 📬 联系我们

如需商务合作或社区支持，请扫描下方二维码添加官方微信：

申请添加好友时，请备注产品名称，比如：“我关注PowerX”

<img src="https://powerx.artisan-cloud.com/images/wx-qr-code.jpg" alt="PowerX 微信二维码" width="220" />

---

## 🤝 贡献指南

我们欢迎所有形式的贡献！

- 提交 [Issue](https://github.com/ArtisanCloud/PowerX/issues) 反馈问题
- 提交 [Pull Request](https://github.com/ArtisanCloud/PowerX/pulls) 贡献代码
- 查看 [贡献指南](./CONTRIBUTING.md) 了解更多细节

### 贡献者

感谢所有为 PowerX 做出贡献的开发者！ 🙏

---

## 📄 许可证

本项目采用 Apache 2.0 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情

---

## 🏆 致谢

感谢所有为 PowerX 项目提供支持的开发者、设计师和用户！

---

**⭐ 如果这个项目对您有帮助，请给我们一个 Star！**

👉 **一句话总结**：**PowerX 是企业级 AgentOS（Agent Operating System），让 CRM、SCRM、电商、审批流等业务模块以 AI 智能体形式共存和协作，通过 MCP/gRPC/REST 多协议统一，实现自主学习与进化，大幅降低运维成本并提供标准化扩展点。**
