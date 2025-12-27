import { watch } from "vue";
import { useColorMode } from "#imports";

type ThemePreference = "light" | "dark" | "system";

const coercePreference = (value?: string | null): ThemePreference => {
  const normalized = String(value ?? "").trim().toLowerCase();
  if (normalized === "dark" || normalized === "light") return normalized;
  if (normalized === "system" || normalized === "auto") return "system";
  return "system";
};

const resolveTheme = (preference: ThemePreference, actual: string): "light" | "dark" => {
  if (preference === "dark") return "dark";
  if (preference === "light") return "light";
  return actual === "dark" ? "dark" : "light";
};

export default defineNuxtPlugin(() => {
  if (!process.client) return;

  const colorMode = useColorMode();

  const syncDom = () => {
    const preference = coercePreference(colorMode.preference);
    const actual = resolveTheme(preference, colorMode.value);
    const root = document.documentElement;

    root.dataset.theme = actual;
    root.setAttribute("data-color-mode", preference);
    root.classList.toggle("dark", actual === "dark");
    root.classList.toggle("light", actual === "light");
  };

  const announce = (preference: ThemePreference) => {
    window.dispatchEvent(
      new CustomEvent("theme-changed", {
        detail: preference,
      })
    );
  };

  watch(
    () => [coercePreference(colorMode.preference), colorMode.value],
    () => syncDom(),
    { immediate: true }
  );

  watch(
    () => coercePreference(colorMode.preference),
    (pref) => announce(pref),
    { immediate: true }
  );
});
