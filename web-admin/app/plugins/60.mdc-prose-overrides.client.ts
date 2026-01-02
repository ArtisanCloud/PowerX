import ProsePre from "~/components/prose/ProsePre.vue";
import ProseA from "~/components/prose/ProseA.vue";

export default defineNuxtPlugin((nuxtApp) => {
  // MDCRenderer 在运行时通过 resolveComponent("ProsePre") 渲染代码块；
  // Nuxt 的 components auto-import 是编译期能力，无法被 resolveComponent 直接发现，
  // 因此需要在这里把覆盖组件注册到 vueApp 的全局组件表里。
  nuxtApp.vueApp.component("ProsePre", ProsePre);
  nuxtApp.vueApp.component("ProseA", ProseA);
});
