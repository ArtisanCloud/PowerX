export default defineNuxtPlugin(() => {
  const router = useRouter()
  router.beforeEach((to, from) => {
    console.info('[PXAdmin][ROUTER→]', to.fullPath, 'from', from.fullPath || '(entry)')
  })
  router.afterEach((to) => {
    console.info('[PXAdmin][ROUTER✓]', to.fullPath)
  })
})
