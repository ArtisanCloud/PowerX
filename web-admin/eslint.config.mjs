// @ts-check
// 兼容 .nuxt 目录尚未生成时（如首次启动、CI 安装阶段）
// 使用动态导入并降级为直通配置，避免 ERR_MODULE_NOT_FOUND。

let withNuxt
try {
  withNuxt = (await import('./.nuxt/eslint.config.mjs')).default
} catch {
  // .nuxt 未生成时，降级为直通配置
  withNuxt = /** @param {import('eslint').Linter.FlatConfig[]|undefined} cfg */ (cfg) => (Array.isArray(cfg) ? cfg : [])
}

export default withNuxt(
  // Your custom configs here
)
