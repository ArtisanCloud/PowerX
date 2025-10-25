// // plugins/redirect-p-to-plugins.client.ts
// export default defineNuxtPlugin(() => {
//   const router = useRouter()
//   router.beforeEach((to) => {
//     // 只拦“顶层路由”跳到了 /_p/** 的情况 → 立即改为 /_plugins/**
//     if (to.fullPath.startsWith('/_p/')) {
//       const clean = to.fullPath.replace(/^\/_p\//, '/_plugins/')
//       console.warn('[PXAdmin][TOP] redirect /_p →', clean)
//       return navigateTo(clean, { replace: true })
//     }
//   })
// })
