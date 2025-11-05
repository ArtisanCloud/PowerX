# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PowerX is a **modular, extensible, pluggable business engine**. It's not a traditional monolithic application but rather a "business kernel + plugin marketplace" architecture. The system enables CRM, e-commerce, SCRM, and other business modules to coexist as plugins on a unified foundation.

**Core Philosophy:**
- **Minimal Kernel**: Core provides only universal capabilities (IAM, RBAC, Event Bus, Audit, DB Layer, Flow Engine)
- **Plugin-First**: All business functionality lives as plugins with independent deployment and database schemas
- **Contract-Driven**: Plugins communicate via contracts/interfaces and event topics, not direct dependencies

## Repository Structure

### Backend (Go)
```
/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/backend/
├── cmd/                    # Binaries (agent/, demo/, test_agent_api/)
├── api/                    # HTTP/gRPC endpoints and handlers
├── internal/               # Application modules (not for external reuse)
│   ├── app/                # Application layer
│   ├── bootstrap/          # Bootstrap configuration
│   ├── contract/           # Contract definitions
│   ├── infra/              # Infrastructure layer
│   │   ├── database/       # Database abstractions (GORM)
│   │   ├── auth/           # Authentication/Authorization
│   │   ├── plugin/         # Plugin system
│   │   ├── cache/          # Caching (Redis)
│   │   ├── media/          # File storage (S3/MinIO)
│   │   └── transport/      # HTTP/gRPC/WebSocket transport
│   └── transport/          # Transport layer
├── pkg/                    # Reusable libraries
│   ├── corex/              # Core SDK capabilities
│   │   ├── rbac/           # RBAC implementation
│   │   └── rls/            # Row-level security
│   ├── auth/               # Auth utilities
│   ├── plugin_mgr/         # Plugin management
│   ├── comm/               # WebSocket/SSE communication
│   └── utils/              # Utilities
├── domain/                 # Domain models and logic
├── config/                 # Configuration loading/validation
├── storage/                # Storage interfaces
├── tests/                  # Test suites
├── specs/                  # Specification documents
└── plugins/, extensions/   # Optional plugin implementations
```

### Frontend (Nuxt)
```
/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/web-admin/
├── app/                    # Nuxt pages and components
├── public/                 # Static assets
├── nuxt.config.ts          # Nuxt configuration
├── package.json            # Dependencies (Nuxt 4, TypeScript, ECharts, Pinia)
└── .env                    # Environment variables
```

## Common Commands

### Backend Development
```bash
# Development
make dev                    # Run demo server (port 8077, logs to stdout)
make dev-agent              # Run agent service
make dev-watch              # Hot reload (requires 'air' tool)

# Build
make build                  # Build all binaries to bin/
make build-agent            # Build agent service
make build-demo             # Build demo service
make build-cross            # Cross-compile for Linux/Windows/macOS
make build-release          # Release build with version tags

# Testing
make unit-test              # Run Go unit tests (go test ./...)
make unit-test-eino         # Run Eino agent tests specifically
make test-all               # Full API test suite (requires running server)
make test-health            # Health check test
make test-quick             # Quick health + config tests
make test-coverage          # Generate coverage report (reports/coverage.html)

# Database
make db-migrate             # Run database migrations
make db-rollback            # Rollback migrations
make db-seed                # Seed database with initial data
make db-refresh             # Rollback + migrate + seed
make db-status              # Check migration status

# Code Quality
make format                 # Format code (go fmt)
make vet                    # Static analysis (go vet)
make check-all              # Full lint check (requires golangci-lint)
make generate               # Run go generate
make docs-api               # Generate Swagger docs (requires swag)

# Utilities
make deps-tidy              # Sync go.mod/go.sum
make logs-tail              # Follow application logs
make profile                # Start pprof (visit http://localhost:8077/debug/pprof/)
make install-tools          # Install dev tools (air, golangci-lint, swag)
```

### Frontend Development
```bash
cd web-admin
npm install                 # Install dependencies
npm run dev                 # Start dev server (port 3000)
npm run build               # Build for production
npm run preview             # Preview production build
```

### Environment Variables
```bash
# Backend
DEV_PORT=8077               # Server port (default: 8077)
LOG_LEVEL=debug             # Logging level
PGHOST, PGPORT, PGUSER      # PostgreSQL connection
PGDATABASE=corex            # Database name

# Frontend
NITRO_PORT=3000             # Nuxt dev server port
```

## Key Components

### 1. Plugin System
- Plugins are discovered at startup from `/plugins` directory
- Each plugin has `plugin.yaml`, executable, and frontend resources
- Communication via Event Bus or contract interfaces
- Admin UI dynamically loads plugin menus via `/api/v1/admin/manifest`

### 2. Core Capabilities (pkg/corex/*)
- **IAM**: Users, departments, roles, tags
- **RBAC**: Role-based access control with plugin-level resources
- **Event Bus**: Local/Redis implementation for plugin communication
- **Audit**: Unified audit logging
- **DB Layer**: Multi-tenant with plugin-independent schemas
- **Flow Engine**: Orchestrated execution flows (Plan/Task/Node)

### 3. Transport Layer
- **HTTP**: RESTful APIs with Swagger documentation
- **gRPC**: High-performance RPC
- **WebSocket/SSE**: Real-time communication

### 4. Agent Lifecycle & Observability
- Full lifecycle management via HTTP/gRPC (register, activate, pause, scale, retire)
- Built-in health scoring, trend queries, subscription filtering
- Enterprise IM alerting with 13-month retention
- Unit/integration tests: `go test ./tests/unit/agent_lifecycle/...`

## Development Workflow

### Testing
```bash
# 1. Ensure server is running
make dev

# 2. Run API tests in another terminal
make test-all

# 3. Run unit tests
make unit-test

# 4. Generate coverage
make test-coverage
```

### Creating New Features
1. **Backend**: Add to appropriate `internal/` module (e.g., `internal/infra/` for infrastructure)
2. **Contracts**: Define in `internal/contract/` for API schema consistency
3. **Tests**: Place test files alongside code (e.g., `foo_test.go`)
4. **Documentation**: Update Swagger via `swag` annotations in handlers
5. **Frontend**: Build Nuxt components in `web-admin/app/`

### API Documentation
- Swagger UI available at: `http://localhost:8077/api/v1/docs`
- Auto-generate with: `make docs-api` (requires swag tool)
- Use `swag init -g cmd/demo/main.go -o docs/api`

## Configuration

- **Config File**: `etc/config.yaml` for core service parameters
- **Environment**: Copy `.env.sample` to `.env` to override with environment variables
- **Database**: PostgreSQL (configurable via env vars)
- **Cache**: Redis
- **Storage**: MinIO/S3 for media files

## Important Implementation Details

### 1. Plugin Development
- Plugins use `corex` SDK for kernel capabilities
- Define contracts in `internal/contract/` for type safety
- Subscribe to events instead of direct dependencies
- Independent database schema per plugin

### 2. Database
- Uses GORM with PostgreSQL driver
- Multi-tenant with tenant isolation
- Migrations via custom tool: `go run ./backend/cmd/database`
- Plugin schemas are independent but share DB instance

### 3. Testing Strategy
- **Unit tests**: `go test ./...` for all packages
- **API tests**: HTTP calls to running server (`make test-all`)
- **Integration tests**: `make integration-test` (starts server + runs tests)
- Coverage reports in `reports/coverage.html`

### 4. Build Process
- Multiple entry points in `cmd/` (agent, demo, test_agent_api)
- Cross-platform builds: `make build-cross`
- Release builds include version from git tags

## Code Style & Conventions

### Backend (Go)
- Go 1.24+
- Format with `go fmt`, keep imports tidy
- Files: snake_case naming
- Tests: `_test.go` suffix
- Exported identifiers need doc comments
- Prefer small packages, lowercase names, no underscores

### Frontend (Nuxt)
- Nuxt 4 with TypeScript
- Pinia for state management
- ECharts for data visualization
- TailwindCSS for styling

### Communication
- Use **simplified Chinese** for communication, comments, and commit messages
- Generate code with Chinese comments and documentation
- Use bilingual approach when necessary (Chinese + English terminology)

## Commit Guidelines

Use Conventional Commits:
- `feat:` - New features
- `fix:` - Bug fixes
- `chore:` - Maintenance tasks
- `refactor:` - Code refactoring
- `docs:` - Documentation
- `test:` - Testing

Example: `feat(api): add chat stream endpoint`

## Security

- **Never commit secrets** - Use environment variables
- Validate configuration with `make test-verify`
- Use `pkg/make_files/secret.mk` for secret management helpers
- RBAC system controls plugin-level access
- Audit logging for all operations

## Troubleshooting

### Server won't start
```bash
# Check database connection
make db-check

# Check environment
make dev-status

# View logs
make logs-tail
```

### API tests failing
```bash
# Ensure server is running
make dev

# Check server health
make test-health

# Verify configuration
make test-verify
```

### Build failures
```bash
# Clean and rebuild
make clean
make deps-tidy
make build
```

## Tools Installation

Optional but recommended:
```bash
# Hot reload
go install github.com/cosmtrek/air@latest

# Linting
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# API docs
go install github.com/swaggo/swag/cmd/swag@latest

# Debugging
go install github.com/go-delve/delve/cmd/dlv@latest

# Mock generation
go install github.com/golang/mock/mockgen@latest
```

Install all: `make install-tools`
