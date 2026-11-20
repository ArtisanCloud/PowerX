# PowerX

**面向企业应用与智能体的开源底座。**

业务插件可以独立开发、安装和升级，不必各自实现租户、权限和事件系统。插件把业务能力注册到 PowerX，工作流和智能体再按授权调用这些能力。这个仓库是 PowerX Core，包含服务端和管理后台。

[快速开始](#快速开始) · [插件开发](https://powerx-doc.artisan-cloud.com/zh/developers/) · [产品官网](https://artisan-cloud.com/products/powerx-base) · [English](./README-en.md)

![PowerX Core 关系图：业务插件注册能力，工作流和智能体按授权调用](./docs/assets/readme/powerx-core-relationship.svg)

## 快速开始

| 你想做什么 | 从这里开始 |
| --- | --- |
| 在服务器上部署并体验后台 | [用 XDocker 部署](https://powerx-doc.artisan-cloud.com/zh/guides/xdocker.html)，无需在服务器上编译 Go 或 Nuxt |
| 修改 PowerX 源码或本地联调 | [本地开发启动](https://powerx-doc.artisan-cloud.com/zh/guides/local-dev.html)，配置依赖并启动后端和管理后台 |
| 已有 PowerX，想接入自己的业务 | [开发第一个插件](https://powerx-doc.artisan-cloud.com/zh/developers/)，从本地初始化到安装验证 |

第一次跑起来，可以先登录管理后台，打开「开放市场 → 插件市场」，看看当前环境有哪些插件、版本和启用状态。

## 产品实景：插件市场

![PowerX 插件市场：查看插件版本与启用状态](https://powerx-doc.artisan-cloud.com/images/readme/zh/plugin-market.png)

*插件市场界面示例，展示插件的版本和启用状态。截图中的插件不代表默认安装或随 Core 开源。*

## 从一个业务问题开始

> **这周，哪些客户需要跟进？**

1. **查客户上下文**：查看插件里的客户资料、最近沟通和已有待办；智能体可按权限整理重点。
2. **团队确认**：负责人判断哪些客户需要跟进，确认优先级和下一步。
3. **创建跟进任务**：流程根据确认结果生成待办，明确谁负责、何时跟进。

这是[业务场景示意](https://artisan-cloud.com/products/powerx-base)，不是开箱即用的跨插件演示。实际能用哪些数据、能执行哪些动作，取决于安装的插件、配置和授权。

## 底座提供什么

- **身份与权限**：统一租户、成员、角色和权限，让插件复用同一套访问规则。
- **插件与集成**：管理插件的安装、启停和升级，通过集成网关、能力契约和事件连接业务系统。
- **工作流与智能体**：用任务调度、模型和知识能力编排业务步骤，按授权调用插件，并查看运行记录。

应用的业务规则和界面留在插件里。先做一个能运行的插件，再把需要复用的业务动作注册为能力，不用先改内核。参见[插件开发指南](https://powerx-doc.artisan-cloud.com/zh/developers/)和[插件与方案](https://powerx-doc.artisan-cloud.com/zh/plugins/)。

## 仓库导航

| 目录 | 用途 |
| --- | --- |
| [`backend/`](./backend) | Go 服务端、API、数据迁移和插件运行时 |
| [`web-admin/`](./web-admin) | Nuxt 4 管理后台 |
| [`deploy/`](./deploy) | 部署相关配置 |
| [`docs/`](./docs) · [`specs/`](./specs) | 仓库内的开发运维说明、功能规格与验收 |

有可复现的问题，欢迎提交 [Issue](https://github.com/ArtisanCloud/PowerX/issues)；业务合作请走[官网](https://artisan-cloud.com/products/powerx-base)。

## 许可证

PowerX Core 采用 [Apache License 2.0](./LICENSE)。插件独立发布，许可不随 Core 继承；CRM、MediaXStudio 等商业插件不随核心一起开源，功能、交付及二次开发权限以各插件的版本和授权为准。
