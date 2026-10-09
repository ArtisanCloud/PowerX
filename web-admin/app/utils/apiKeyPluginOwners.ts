/** Explicit exact owners; an empty list revokes plugin ownership. */
export function parseApiKeyPluginOwners(text: string): string[] {
  const ids = text.split(/[\n,]+/).map((item) => item.trim()).filter(Boolean);
  if (ids.length > 32) throw new Error("API_KEY_PLUGIN_OWNER_LIMIT");
  if (ids.some((id) => !/^[a-z0-9][a-z0-9._-]{0,127}$/.test(id))) {
    throw new Error("API_KEY_PLUGIN_OWNER_INVALID");
  }
  return [...new Set(ids)].sort();
}
