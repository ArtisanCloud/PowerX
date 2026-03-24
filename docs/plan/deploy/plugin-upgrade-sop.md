# 插件手动安装与平滑升级 SOP（无市场阶段）

## 1. 流程原则

- 禁止“直接覆盖升级”
- 统一采用：**安装新版本不启用 -> 健康验证 -> switch_version 启用**
- 始终保留 N-1 版本用于快速回滚

## 2. 前置检查

- 后端服务健康：`GET /api/v1/health`
- 目标插件当前状态：`GET /api/v1/admin/plugins/:id/status`
- 插件包可用（目录或离线包解压目录包含 `plugin.yaml`）

## 3. 标准升级步骤

### Step 1：安装新版本（不启用）

```bash
curl -X POST http://127.0.0.1:8077/api/v1/admin/plugins/install/local \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <ADMIN_TOKEN>' \
  -d '{
    "src_dir": "/data/plugin_pkgs/com.powerx.demo/v1.2.3",
    "enable": false,
    "force": false,
    "metadata": {"scope":"prod","environment":"production"}
  }'
```

### Step 2：检查安装结果

- `GET /api/v1/admin/plugins/:id` 确认新版本已存在
- `GET /api/v1/admin/plugins/:id/status` 确认当前运行仍是旧版本

### Step 3：执行版本切换

```bash
curl -X POST http://127.0.0.1:8077/api/v1/admin/plugins/com.powerx.demo/switch_version \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <ADMIN_TOKEN>' \
  -d '{"version":"v1.2.3","enable":true}'
```

### Step 4：切换后观察

- `GET /api/v1/admin/plugins/com.powerx.demo/status`
- 核心接口冒烟验证
- 观察 `plugin_release.*` 与错误日志 10~30 分钟

## 4. 回滚 SOP

当新版本异常时，直接回切旧版本：

```bash
curl -X POST http://127.0.0.1:8077/api/v1/admin/plugins/com.powerx.demo/switch_version \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <ADMIN_TOKEN>' \
  -d '{"version":"v1.2.2","enable":true}'
```

如需停止故障版本：

```bash
curl -X POST http://127.0.0.1:8077/api/v1/admin/plugins/com.powerx.demo/uninstall \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <ADMIN_TOKEN>' \
  -d '{"version":"v1.2.3","purge":false}'
```

## 5. 禁止事项

- 不经验证直接 `enable=true` 安装并上线
- 无回滚版本时执行生产切换
- 在高峰期同时切多个高风险插件

## 6. 运行保障建议

- 设置 `CORE_X_PLUGIN_AUTORESTORE_PARALLELISM=2~4`
- 升级窗口内开启更高日志级别并采集审计事件
- 将插件包与版本元数据统一归档（包体、checksum、安装人、安装时间）

