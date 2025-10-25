export default defineNuxtPlugin(() => {
  const router = useRouter()
  router.beforeEach((to, from) => {
    console.log('[PXAdmin][ROUTER→]', to.fullPath, 'from', from.fullPath || '(entry)')
  })
  router.afterEach((to) => {
    console.log('[PXAdmin][ROUTER✓]', to.fullPath)
  })
})
