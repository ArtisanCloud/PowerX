# Web Admin 常见故障排查

## 刷新后卡死 / 一直 Loading

如果浏览器控制台出现类似错误：

- `Uncaught (in promise) Error: Could not establish connection. Receiving end does not exist.`
- `The message port closed before a response was received.`

通常原因是**浏览器扩展**向页面注入脚本，并通过 message channel 与自身通信；在刷新/路由切换时 receiver 不存在，就会抛出上述错误。该错误本身与 PowerX 功能无关，但可能触发前端全局错误处理，导致 Loading 状态无法正确结束。

处理建议：

1. 先在无痕窗口打开 Web Admin（默认禁用大多数扩展）验证是否消失。
2. 若确认是扩展导致，逐个禁用扩展定位（常见：翻译/脚本管理/隐私保护/代理/开发辅助类扩展）。
3. 开发环境可使用“硬刷新（Disable cache + Reload）”避免旧脚本残留。

PowerX Web Admin 已对这些“已知扩展错误”做了忽略处理（不再让其导致页面卡死），但最佳实践仍是：在调试期尽量关闭不必要的扩展，以免干扰排查。

