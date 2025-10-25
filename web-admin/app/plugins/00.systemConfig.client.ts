// app/plugins/app-init.client.ts
export default defineNuxtPlugin((nuxtApp) => {
  const { public: pub } = useRuntimeConfig();

  const lang = pub.forceLanguage ?? pub.defaultLanguage ?? "zh";
  const theme = pub.forceTheme ?? pub.defaultTheme ?? "auto"; // 'dark'|'light'|'auto'
  // 让 i18n 不被 cookie 顶回去
  document.cookie = "i18n_redirected=; Max-Age=0; path=/";

  // 为避免初始化竞态，等应用挂载后再“一锤定音”
  nuxtApp.hook("app:mounted", async () => {
    try {
      // 使用 Nuxt 的 i18n 实例来设置语言
      const { $i18n } = nuxtApp as any;
      if ($i18n && typeof $i18n.setLocale === "function") {
        await $i18n.setLocale(lang);
      } else if ($i18n && $i18n.locale) {
        $i18n.locale.value = lang;
      }
    } catch (e) {
      console.error("[init] setLocale failed:", e);
    }
    document.documentElement.lang = lang;

    type ThemePreference = "light" | "dark" | "system";
    const coerceTheme = (input?: string | null): ThemePreference | undefined => {
      const value = String(input ?? "").trim().toLowerCase();
      if (!value) return undefined;
      if (value === "dark" || value === "light") return value;
      if (value === "system" || value === "auto") return "system";
      return undefined;
    };

    const colorMode = useColorMode();
    const themeState = useState<ThemePreference>("theme", () =>
      coerceTheme(colorMode.preference) ?? "system"
    );

    const applyThemePreference = (next: ThemePreference) => {
      themeState.value = next;
      colorMode.preference = next;
    };

    const forcedTheme = coerceTheme(pub.forceTheme);
    const storedTheme =
      coerceTheme(themeState.value) ?? coerceTheme(colorMode.preference);
    const defaultTheme = coerceTheme(theme);
    const desiredTheme =
      forcedTheme ?? storedTheme ?? defaultTheme ?? ("system" as ThemePreference);

    if (pub.forceTheme) {
      applyThemePreference(forcedTheme);
    } else if (desiredTheme) {
      applyThemePreference(desiredTheme);
    }

    if (process.client) {
      watch(
        () => coerceTheme(colorMode.preference) ?? "system",
        (pref) => {
          if (themeState.value !== pref) {
            themeState.value = pref;
          }
        },
        { immediate: true }
      );
    }

    if (pub.debugMode) {
      console.log("🎯 init applied:", {
        lang,
        theme,
        htmlClass: document.documentElement.className,
      });
    }
  });
});
