type CookieRef<T> = { value: T | null };

const cookieStore = new Map<string, CookieRef<any>>();

export function useCookie<T = string | null>(name: string) {
  if (!cookieStore.has(name)) {
    cookieStore.set(name, { value: null });
  }
  return cookieStore.get(name) as CookieRef<T>;
}

export function __resetCookies() {
  cookieStore.clear();
}
