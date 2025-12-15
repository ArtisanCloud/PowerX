import { useCookie } from "./nuxt-app-shim";

export { useCookie };

export function useI18n() {
  return {
    t: (key: string, vars?: Record<string, unknown>) => {
      if (!vars) {
        return key;
      }
      return `${key}:${JSON.stringify(vars)}`;
    },
  };
}

export function useRuntimeConfig() {
  return {
    public: {},
  };
}
