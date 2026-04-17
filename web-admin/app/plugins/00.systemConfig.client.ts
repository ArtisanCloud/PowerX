import { watch } from "vue";
import { useColorMode } from "#imports";

// app/plugins/app-init.client.ts
export default defineNuxtPlugin((nuxtApp) => {
  const { public: pub } = useRuntimeConfig();

  const defaultLang = pub.defaultLanguage ?? "zh";
  const defaultTheme = pub.defaultTheme ?? "auto"; // 'dark'|'light'|'auto'

  const readCookie = (name: string) => {
    if (typeof document === "undefined") return "";
    const match = document.cookie.match(
      new RegExp(`(?:^|;\\s*)${name}=([^;]+)`)
    );
    return match ? decodeURIComponent(match[1]) : "";
  };

  const getStoredLocale = () => {
    if (typeof window === "undefined") return "";
    try {
      return localStorage.getItem("px_locale") || "";
    } catch {
      return "";
    }
  };

  const persistLocale = (value: string) => {
    if (typeof document !== "undefined") {
      document.cookie = `px_lang=${encodeURIComponent(
        value
      )}; path=/; SameSite=Lax; Max-Age=31536000`;
    }
    if (typeof window !== "undefined") {
      try {
        localStorage.setItem("px_locale", value);
      } catch {}
    }
  };

  const persistTheme = (value: ThemePreference) => {
    if (typeof document !== "undefined") {
      document.cookie = `powerx-color-mode=${encodeURIComponent(
        value
      )}; path=/; SameSite=Lax; Max-Age=31536000`;
    }
    if (typeof window !== "undefined") {
      try {
        localStorage.setItem("powerx-color-mode", value);
      } catch {}
    }
  };

  const getStoredTheme = () => {
    if (typeof window === "undefined") return undefined;
    try {
      return (
        localStorage.getItem("powerx-color-mode") ||
        readCookie("powerx-color-mode") ||
        undefined
      );
    } catch {
      return undefined;
    }
  };

  const applyLocale = async () => {
    try {
      const { $i18n } = nuxtApp as any;
      const storedLocale =
        (!pub.forceLanguage && getStoredLocale()) || readCookie("px_lang");
      const desiredLocaleRaw =
        pub.forceLanguage || storedLocale || defaultLang || "zh";
      const normalizeLocale = (input: string) => {
        const raw = String(input || "").trim();
        const base = raw.split("-")[0] || raw;
        const allowed = String(pub.availableLanguages || "zh,en,ja,ko")
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean);
        if (allowed.includes(base)) return base;
        if (allowed.includes(raw)) return raw;
        return defaultLang || "zh";
      };
      const desiredLocale = normalizeLocale(desiredLocaleRaw);
      if ($i18n && typeof $i18n.setLocale === "function") {
        await $i18n.setLocale(desiredLocale);
      } else if ($i18n && $i18n.locale) {
        $i18n.locale.value = desiredLocale;
      }
      persistLocale(desiredLocale);
      document.documentElement.lang = desiredLocale;
      if (process.client) {
        watch(
          () => ($i18n?.locale ? $i18n.locale.value : desiredLocale),
          (val) => persistLocale(normalizeLocale(String(val))),
          { immediate: true }
        );
      }
    } catch (e) {
      console.error("[init] setLocale failed:", e);
    }
  };

  const applyTheme = () => {
    type ThemePreference = "light" | "dark" | "system";
    const coerceTheme = (input?: string | null): ThemePreference | undefined => {
      const value = String(input ?? "").trim().toLowerCase();
      if (!value) return undefined;
      if (value === "dark" || value === "light") return value;
      if (value === "system" || value === "auto") return "system";
      return undefined;
    };

    const colorMode = useColorMode();
    const storedTheme = !pub.forceTheme
      ? coerceTheme(getStoredTheme())
      : undefined;
    const themeState = useState<ThemePreference>("theme", () =>
      storedTheme ?? "system"
    );

    const applyThemePreference = (next: ThemePreference) => {
      themeState.value = next;
      colorMode.preference = next;
      persistTheme(next);
    };

    const forcedTheme = coerceTheme(pub.forceTheme);
    const defaultPref = coerceTheme(defaultTheme) ?? "system";
    const desiredTheme = forcedTheme ?? storedTheme ?? defaultPref;

    applyThemePreference(desiredTheme);

    if (process.client) {
      watch(
        () => coerceTheme(colorMode.preference) ?? "system",
        (pref) => {
          if (themeState.value !== pref) {
            themeState.value = pref;
            persistTheme(pref);
          }
        },
        { immediate: true }
      );
    }
  };

  const run = () => {
    applyLocale();
    applyTheme();

    if (pub.debugMode) {
      console.info("🎯 init applied:", {
        lang: defaultLang,
        theme: defaultTheme,
        htmlClass: document.documentElement.className,
      });
    }
  };

  if (process.client) {
    run();
  } else {
    nuxtApp.hook("app:mounted", run);
  }
});
