package contract

// MediaRequestMaxBytes 为图片任务及能力调用信封保留 Base64 和元数据空间。
// 单图和总图片字节限制另行执行，不能以 HTTP 请求大小代替图片校验。
const MediaRequestMaxBytes = 32 << 20
