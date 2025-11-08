# PowerX – Enterprise-Grade AgentOS (Agent Operating System)

[![中文](https://img.shields.io/badge/中文-自述文件-red)](./README.md)
[![Official Docs](https://img.shields.io/badge/Official_Docs-powerx.artisan--cloud.com-green)](https://powerx.artisan-cloud.com)
[![License](https://img.shields.io/badge/License-Apache%202.0-red.svg)](LICENSE)

> PowerX is an enterprise-grade **Agent Operating System (AgentOS)** centered on **plugin-based intelligent agents**. Through AI Agent orchestration, plugin ecosystem, and unified protocols, it enables enterprise business modules (CRM, e-commerce, SCRM, approval workflows, etc.) to **collaborate and evolve autonomously** as intelligent agents.
>
> **PowerX = AI Agent Orchestration Kernel + Plugin Marketplace + MCP/gRPC/REST Multi-Protocol Unity**

---

## 🌟 Screenshots

### Management Dashboard
![Management Dashboard](https://raw.githubusercontent.com/ArtisanCloud/PowerXDoc/dev/michaelhu/docs/website/public/images/px-home-en.png)

### Plugin Marketplace
![Plugin Marketplace](https://raw.githubusercontent.com/ArtisanCloud/PowerXDoc/dev/michaelhu/docs/website/public/images/px-market-en.png)

---

## 🏗️ Architecture Philosophy

PowerX is an **enterprise-grade AgentOS (Agent Operating System)** following three core principles:

### 1. Minimal Kernel

- The kernel only provides universal capabilities: Identity & Organization (IAM), Role-Based Access Control (RBAC), Event Bus, Audit, Database Abstraction (DB Layer), and Runtime Flow Engine
- These capabilities are exposed as an SDK (`pkg/corex/*`), which can be reused by any plugin and external system

### 2. Agent-First

- All business functionality (e-commerce, SCRM, approval workflows, live streaming, etc.) exists as **AI Agent intelligent agents**
- Agents connect to the kernel through **Contracts**, enabling independent development, deployment, and database schemas
- Agents can be developed by official/3rd-party/customer teams and distributed through the **Plugin Marketplace**, supporting autonomous learning and collaboration

### 3. Contract-Driven + AI-Native

- Plugins do **not directly depend** on each other, but are decoupled through **Contract Interfaces / Event Subscriptions**
- Supports **MCP (Model Context Protocol)** for standardized AI tool integration
- All external interfaces (HTTP/gRPC), events (Event Topics), and data models (OpenAPI/Proto) are centrally managed in the `/contracts` directory
- Built-in toolchains automatically generate SDKs (Go/TypeScript), ensuring frontend-backend consistency

---

## 🔑 Core Components

| Component | Description | Key Features |
|-----------|-------------|--------------|
| **IAM (Identity & Organization)** | Users, departments, roles, tags | Shared organizational architecture across all plugins |
| **RBAC (Role-Based Access Control)** | Unified permission model (Role/Policy) | Extensible to plugin level: plugins only declare "resources and actions", authorization managed by the kernel |
| **Event Bus** | Primary communication mechanism between plugins | Supports Local/Redis implementation, plugins only subscribe to Topics without caring about message sources |
| **Audit (Audit Logs)** | Unified collection of all events/operations | Kernel-level extensibility for compliance (security/risk control) |
| **DB Layer (Database Abstraction)** | Multi-tenant isolation (Tenant) | Plugin-independent schemas but shared Postgres/MySQL instances, unified migration tool (Goose compatible) |
| **Flow (Business Process Engine)** | Built-in orchestratable execution flows (Plan/Task/Node) | Plugins can mount custom Flow nodes |
| **Agent Lifecycle & Observability** | Complete control plane covering agent registration, activation, pause, scaling, and retirement (HTTP / gRPC) | Built-in health score aggregation, trend queries, subscription filtering, and enterprise IM alerting with 13-month retention |

---

## 🔌 Plugin Mechanism

### Plugin Structure
```
plugin-package/
├── plugin.yaml          # Plugin metadata
├── backend/             # Backend executable
│   └── main
├── web-admin/           # Frontend resources
│   ├── pages/
│   └── assets/
└── contract/            # Contract definitions
    └── api.yaml
```

### Plugin Lifecycle
- **Install**: Place plugin packages in the `/plugins` directory
- **Register**: System automatically scans and registers plugins at startup
- **Load**: Dynamically load plugin menus and pages
- **Communicate**: Collaborate with other plugins through Event Bus or Contract Interfaces

---

## 🖥️ Multi-Frontend Support

PowerX includes four types of frontend shells, sharing **unified contracts** with auto-generated SDKs, supporting multiple protocols and frameworks:

| Frontend/Protocol | Use Case | Features |
|-------------------|----------|----------|
| **Admin Dashboard** | Operations/management personnel | Dynamic menu and plugin page loading |
| **Web User Frontend** | C-end user interface | Responsive design, multi-platform support |
| **MiniApp** | Lightweight engagement scenarios | WeChat/Alipay mini-program support |
| **OpenAPI** | Third-party systems | RESTful API unified calling interface |
| **MCP (Model Context Protocol)** | AI Agent and plugin interaction | Standardized AI tool integration protocol |
| **gRPC** | High-performance inter-service communication | Protobuf-based efficient RPC |

---

## 📚 Documentation Resources

For detailed installation, deployment, and usage guides, please refer to:

- **🚀 [Official Docs - Quick Start](https://powerx.artisan-cloud.com)** - Complete installation and deployment guide
- **📖 [API Documentation](https://powerx.artisan-cloud.com/api-docs)** - Interface specifications and examples
- **🏗️ [Architecture](https://powerx.artisan-cloud.com/architecture)** - Deep dive into system design
- **🔌 [Plugin Development Guide](https://powerx.artisan-cloud.com/plugin-development)** - Develop custom plugins
- **☁️ [Deployment Guide](https://powerx.artisan-cloud.com/deployment)** - Production deployment solutions
- **🛠️ [Operations Manual](https://powerx.artisan-cloud.com/operations)** - Monitoring, backup, and upgrades

---

## 🤝 Contributing

We welcome all forms of contribution!

- Submit [Issues](https://github.com/ArtisanCloud/PowerX/issues) to report problems
- Submit [Pull Requests](https://github.com/ArtisanCloud/PowerX/pulls) to contribute code
- See [Contributing Guidelines](./CONTRIBUTING.md) for more details

### Contributors
Thanks to all developers who contribute to PowerX! 🙏

---

## 📄 License

This project is licensed under the Apache 2.0 License - see the [LICENSE](LICENSE) file for details

---

## 🏆 Acknowledgments

Thanks to all developers, designers, and users who support the PowerX project!

---

**⭐ If this project is helpful to you, please give us a Star!**

👉 **One-line Summary**: **PowerX is an enterprise-grade AgentOS (Agent Operating System) that enables business modules like CRM, SCRM, e-commerce, and approval workflows to coexist and collaborate as AI intelligent agents through MCP/gRPC/REST multi-protocol unity, achieving autonomous learning and evolution while significantly reducing operational costs and providing standardized extension points.**
