# 插件生命周期与 drain 规则

## 核心原则

`drain` 是停用、卸载、切换版本、强制重装之前的安全收敛手段，不是用户主动作。页面不能裸露一个没有目标的“发起 drain”作为主要操作。

用户动作只有：
- 系统启用
- 系统停用
- 租户订阅启用
- 租户取消订阅
- 切换版本
- 卸载
- 最终卸载

后端必须 fail-fast：只要目标插件存在 active 或 ready drain job，就禁止新增使用入口，包括租户订阅、插件业务写入、插件 owned Event Fabric topic/resource 创建、插件 owned Scheduler Job 创建。

## 状态定义

### 插件系统状态

| 状态 | 含义 | 允许动作 |
| --- | --- | --- |
| 未安装 | 系统无该插件安装记录 | 安装 |
| 已安装未启用 | 插件包已落地，运行时未启动 | 系统启用、卸载 |
| 已启用 | 运行时已启动，路由/菜单可挂载 | 系统停用、重启、切换版本、卸载、租户订阅 |
| drain 中 | 已阻断新增使用入口，等待存量任务/实例退出 | 查看阻断、管理阻断任务、刷新状态 |
| ready_to_uninstall | drain 已完成，等待目标动作 | 卸载 drain: 最终卸载；停用 drain: 系统停用 |

### drain job 状态

| job 状态 | 是否阻断新增使用 | 页面主动作 |
| --- | --- | --- |
| requested | 是 | 等待 drain 完成 |
| blocking_new_usage | 是 | 等待 drain 完成 |
| draining | 是 | 查看/管理阻断任务 |
| ready_to_uninstall | 是 | 按 reason 分流：最终卸载或停用 |
| completed | 否 | 历史记录，不阻断 |
| failed/cancelled | 否 | 历史记录，不阻断 |

`ready_to_uninstall` 虽然名字包含 uninstall，但它表达的是“drain 已 ready”。页面和后端要结合 `reason` 判断下一步：
- `root uninstall requested`：允许最终卸载。
- `root disable requested`：允许系统停用。
- 其他 manual reason：按卸载前 drain 处理，不允许新增使用。

## 页面动作矩阵

| 场景 | 顶部按钮 | 系统运行卡片 | 租户卡片 |
| --- | --- | --- | --- |
| 未安装 | 安装 | 不显示系统运行控制 | 不允许订阅 |
| 已安装未启用，无 drain | 卸载 | 启用 | 不允许新增订阅 |
| 已启用，无 drain | 卸载 | 停用、重启、切换版本 | 订阅启用、取消订阅、轮换凭证、删除配置 |
| drain active | 等待 drain 完成 | 禁止启停、重启、切版本；允许查看/管理阻断任务 | 禁止新增订阅；已订阅租户允许取消订阅或删除配置 |
| ready + uninstall/manual | 最终卸载 | 禁止启用、停用、重启、切版本 | 禁止新增订阅；允许删除已有配置 |
| ready + disable | 等待停用 | 允许停用；禁止启用、重启、切版本 | 禁止新增订阅；允许删除已有配置 |

## 后端入口规则

### active tenant binding 定义

`active tenant binding` 只包含仍可能发起插件使用的租户实例：
- `subscribed`
- `enabled`

以下状态不算 active，不应阻断 force 安装、切换版本或系统停用：
- `disabled`
- `expired`
- `drained`
- `available`

发起 drain 时也只能把 active tenant binding 推进 `draining_requested` 或 `disabled_by_platform`。已取消订阅的 disabled 配置不能被 drain job 反向标记为 draining。

### 安装/系统启用

安装并 `enable=true` 或系统启用成功后，可以清理目标插件旧的 `ready_to_uninstall` job 为 `completed`，然后为当前请求租户启用实例并同步 Event Fabric topic。

这一步是显式恢复插件生命周期，必须发生在系统运行时已启用之后。

### 租户订阅启用

租户订阅启用不能清理 `ready_to_uninstall` job。它必须先经过 `EnsurePluginAcceptsNewUsage`：
- 有 active drain job：拒绝。
- 有 `ready_to_uninstall` drain job：拒绝。
- 有租户实例处于 `draining_requested` 或 `disabled_by_platform`：拒绝。

### 系统停用

系统停用必须先满足：
- 没有 active drain job。
- 没有 active tenant plugin binding。

如果存在启用租户实例，前端从“停用”动作引导创建 `reason=root disable requested` 的 drain job；ready 后再调用停用接口。

### 卸载

卸载分两步：
1. 点击“卸载”时，如存在租户实例或运行任务，创建 `reason=root uninstall requested` 的 drain job。
2. drain ready 后点击“最终卸载”，后端删除 drained 租户实例，完成 drain job，并移除系统安装记录。

### 切换版本/强制重装

目标版本替换当前已启用运行时时，若存在 active tenant plugin binding，必须先 drain。不能用 force 绕过 drain。

## 禁止实现

- 禁止前端因为 `sysEnabled=true` 隐藏 `ready_to_uninstall` job。
- 禁止用租户订阅启用入口清理 ready drain job。
- 禁止把“发起 drain”作为顶部主按钮。
- 禁止在 drain active/ready 时新增订阅、轮换凭证、创建插件 owned Event Fabric resource 或 Scheduler Job。
- 禁止为了旧数据兼容绕过 blocking drain job。
