const toHex = (bytes: Uint8Array) =>
  Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");

const toUUIDFromBytes = (bytes: Uint8Array) => {
  if (bytes.length < 16) return "";
  const b = bytes.slice(0, 16);
  b[6] = (b[6] & 0x0f) | 0x50; // version 5
  b[8] = (b[8] & 0x3f) | 0x80; // variant RFC 4122
  const hex = toHex(b);
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20, 32)}`;
};

export const useFileHash = () => {
  const hashFileSHA256 = async (file: File): Promise<string> => {
    const buffer = await file.arrayBuffer();
    const digest = await crypto.subtle.digest("SHA-256", buffer);
    return toHex(new Uint8Array(digest));
  };

  const buildStorageKeyFromFile = async (file: File, scopeKey?: string) => {
    const buffer = await file.arrayBuffer();
    const digest = await crypto.subtle.digest("SHA-256", buffer);
    const bytes = new Uint8Array(digest);
    const sha256 = toHex(bytes);
    if (!scopeKey) {
      return { sha256, uuid: toUUIDFromBytes(bytes) };
    }
    const payload = new TextEncoder().encode(`${scopeKey}:${sha256}`);
    const tenantDigest = await crypto.subtle.digest("SHA-256", payload);
    const tenantBytes = new Uint8Array(tenantDigest);
    return { sha256, uuid: toUUIDFromBytes(tenantBytes) };
  };

  return {
    hashFileSHA256,
    buildStorageKeyFromFile,
  };
};
