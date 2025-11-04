# 静态资源与图像优化

> 指南说明在 PowerX Web Admin 中管理图片、字体和静态资源的最佳实践，借助 Nuxt Image、CDN 缓存与构建策略提升加载性能。

---

## 1. Nuxt Image 方案

- 项目已安装 `@nuxt/image`（`package.json:dependencies`），推荐在组件中使用 `<NuxtImg>` 取代原生 `<img>`：  
  ```vue
  <NuxtImg
    src="/hero/console.png"
    alt="控制台截图"
    width="640"
    height="360"
    format="webp"
    class="rounded-xl shadow-lg"
  />
  ```
- 在 `nuxt.config.ts` 中可配置默认提供商、质量、屏幕断点：  
  ```ts
  image: {
    quality: 80,
    screens: { sm: 640, md: 768, lg: 1024, xl: 1280 },
  }
  ```
- 对于外部图像使用 `providers` 配置（S3、OSS、Imgix 等）。

---

## 2. 优化策略

| 项目 | 建议 | 说明 |
| --- | --- | --- |
| 图像格式 | 优先使用 WebP/AVIF，必要时保留 JPG/PNG 兜底 | `<NuxtImg>` 会自动协商格式 |
| 大图延迟加载 | `loading="lazy"`（Nuxt 默认）或 IntersectionObserver | 对非首屏图像开启懒加载 |
| 响应式尺寸 | 提供 `sizes`/`srcset` 或 `<NuxtImg :sizes="{ md: '50vw', lg: '33vw' }" />` | 减少移动端带宽 |
| SVG | 纯图标使用 Iconify；复杂 SVG 可内联并压缩 | 避免嵌入 base64 |
| 字体 | 使用 `font-display: swap`，通过 `@nuxt/ui` 管理 | 减少 FOIT/FOUT |

---

## 3. 静态资源组织

- 公共静态文件放在 `public/`，访问路径 `/xxx`。  
- 构建产物（打包后）位于 `.output/public`，需要 CDN 配置长缓存（`Cache-Control: public, max-age=31536000, immutable`）。  
- 对频繁更新的文件（如配置 JSON）使用短缓存并附加版本号查询参数。

---

## 4. CDN 与缓存

- 推荐使用 CDN（CloudFront、Cloudflare、OSS CDN）加速静态资源：  
  - 图片启用压缩、WebP 重写。  
  - 设置区域缓存与边缘 TTL。  
  - 对 API 请求保持合理的 `Cache-Control`，避免缓存敏感数据。
- 若部署在 Vercel/NuxtHub，可直接使用内置静态资源优化。

---

## 5. 构建期优化

- 使用 `npm run build --analyze` 查看 bundle 中是否包含体积过大的图片。  
- 将大图片移至 CDN 或改为按需懒加载。  
- 提前压缩图片（ImageOptim、Squoosh、Sharp pipeline）。  
- 对图表、示意图等可使用 Lottie/Canvas 渲染，减少静态资源。

---

## 6. QA Checklist

- [ ] 核心页面 Lighthouse 性能评分 > 90。  
- [ ] 移动端首屏图片大小 < 150 KB。  
- [ ] 所有 `<img>` / `<NuxtImg>` 带有 `alt` 文本。  
- [ ] 静态资源命名含版本号或 hash，避免 CDN 未同步。  
- [ ] Dark 模式下图像对比度符合要求（必要时提供双版本）。

---

## 7. 后续计划

- 引入图像上传服务端处理（Sharp/Cloudinary）自动生成多尺寸。  
- 为工作流节点/插件图标建立 Sprite Sheet 或 Icon Font，减少请求。  
- 在 CI 中加入静态资源体积报警（超过阈值时阻止合并）。  
- 结合 `nuxt-picture`/`nuxt-image` 的 `placeholder="blur"` 提升感知速度。
