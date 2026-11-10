import { createHash } from 'node:crypto';
import { isIP } from 'node:net';

// JSON.parse 会吞掉重复键；共同契约在进入 schema/JCS 前显式拒绝重复键及孤立 surrogate。
export function parseStrictJSON(raw) {
  let i = 0;
  const skip = () => { while (/[\t\r\n ]/.test(raw[i] ?? '\0')) i++; };
  const string = () => {
    const start = i++;
    while (i < raw.length) { if (raw[i] === '\\') { i += 2; continue; } if (raw[i++] === '"') break; }
    const value = JSON.parse(raw.slice(start, i));
    if (!value.isWellFormed()) throw new Error('invalid Unicode');
    return value;
  };
  const value = () => {
    skip();
    if (raw[i] === '"') return string();
    if (raw[i] === '{') {
      i++; skip(); const out = Object.create(null); const keys = new Set();
      if (raw[i] === '}') { i++; return out; }
      while (i < raw.length) {
        skip(); if (raw[i] !== '"') throw new Error('expected property');
        const key = string(); if (keys.has(key)) throw new Error('duplicate property'); keys.add(key);
        skip(); if (raw[i++] !== ':') throw new Error('expected colon'); out[key] = value(); skip();
        if (raw[i] === '}') { i++; return out; } if (raw[i++] !== ',') throw new Error('expected comma');
      }
      throw new Error('unterminated object');
    }
    if (raw[i] === '[') {
      i++; skip(); const out = []; if (raw[i] === ']') { i++; return out; }
      while (i < raw.length) { out.push(value()); skip(); if (raw[i] === ']') { i++; return out; } if (raw[i++] !== ',') throw new Error('expected comma'); }
      throw new Error('unterminated array');
    }
    const match = /^(?:true|false|null|-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)/.exec(raw.slice(i));
    if (!match) throw new Error('invalid JSON'); i += match[0].length;
    const result = JSON.parse(match[0]); if (typeof result === 'number' && !Number.isFinite(result)) throw new Error('non-finite number'); return result;
  };
  const out = value(); skip(); if (i !== raw.length) throw new Error('trailing JSON'); return out;
}

export function canonicalize(value) {
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(canonicalize).join(',')}]`;
  return `{${Object.keys(value).sort().map(key => `${JSON.stringify(key)}:${canonicalize(value[key])}`).join(',')}}`;
}

export function validArtifactPath(value) {
  return typeof value === 'string' && value !== '' && !value.includes('\0') && !/[\\%?#:\r\n\t]/.test(value) && !value.startsWith('/') && value.split('/').every(segment => segment !== '' && segment !== '.' && segment !== '..');
}

export function manifestDigest(manifest) {
  const { build_id: _id, built_at: _time, ...payload } = manifest;
  const paths = new Set();
  for (const artifact of payload.artifacts) {
    if (!validArtifactPath(artifact.path) || paths.has(artifact.path)) throw new Error('invalid or duplicate artifact path'); paths.add(artifact.path);
    if ((payload.component === 'backend') !== (artifact.kind === 'binary')) throw new Error('component and artifact kind mismatch');
  }
  payload.artifacts = [...payload.artifacts].sort((a, b) => Buffer.compare(Buffer.from(a.kind), Buffer.from(b.kind)) || Buffer.compare(Buffer.from(a.path), Buffer.from(b.path)));
  const canonical = canonicalize(payload);
  return { canonical, build_id: createHash('sha256').update(`powerx.build-fingerprint/v1\n${canonical}`).digest('hex') };
}

export function normalizeSite(raw) { return normalizeWebsite(raw, true); }

export function normalizeWebsite(raw, requireHTTPS = false) {
  if (raw.trim() !== raw || raw.includes('\0') || /[\\%\r\n\t?#]/.test(raw)) throw new Error('ambiguous site URL');
  const match = /^(https?):\/\/([^/]+)(.*)$/.exec(raw); if (!match || (requireHTTPS && match[1] !== 'https') || match[2].includes('@')) throw new Error('invalid HTTP site');
  const url = new URL(raw);
  const presentedHost = match[2].startsWith('[') ? match[2].slice(1, match[2].indexOf(']')) : match[2].split(':')[0].toLowerCase();
  if (!isIP(presentedHost) && (url.hostname !== presentedHost || !presentedHost.includes('.') || presentedHost.split('.').some(label => !label || label.startsWith('-') || label.endsWith('-') || label.startsWith('0x')))) throw new Error('ambiguous hostname');
  if (url.pathname.split('/').some(segment => segment === '.' || segment === '..') || match[3].split('/').some(segment => segment === '.' || segment === '..') || match[3].includes('//')) throw new Error('ambiguous site path');
  if (!/^[a-zA-Z0-9.:[\]-]+$/.test(match[2]) || (!url.hostname.includes('.') && !url.hostname.startsWith('['))) throw new Error('ambiguous host');
  if (url.pathname !== (match[3] || '/')) throw new Error('URL parser changed site path');
  url.pathname = url.pathname.replace(/\/+$/, '') + '/'; return url.href;
}

export function remainingMillis(serverTime, expiresAt, expiresIn, rtt, existing) {
  const utcMilliseconds = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z$/;
  if (!utcMilliseconds.test(serverTime) || !utcMilliseconds.test(expiresAt)) throw new Error('invalid UTC milliseconds');
  if (!Number.isSafeInteger(expiresIn) || expiresIn <= 0 || expiresIn > 600000 || !Number.isSafeInteger(rtt) || rtt < 0 || Date.parse(expiresAt) - Date.parse(serverTime) !== expiresIn) throw new Error('inconsistent challenge time');
  return Math.max(0, Math.min(expiresIn - rtt - 1000, existing ?? Infinity));
}
