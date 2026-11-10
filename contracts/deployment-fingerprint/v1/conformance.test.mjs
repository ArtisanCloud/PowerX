import { readFileSync } from 'node:fs';
import test from 'node:test';
import assert from 'node:assert/strict';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { parseStrictJSON, manifestDigest, validArtifactPath, normalizeSite, normalizeWebsite, remainingMillis } from './conformance.mjs';

const read = name => parseStrictJSON(readFileSync(new URL(name, import.meta.url), 'utf8'));
const schema = read('protocol.schema.json');
const ajv = new Ajv2020({ allErrors: true, strict: true }); addFormats(ajv); ajv.addSchema(schema);
for (const row of read('schema-cases.json').cases) test(`schema: ${row.name}`, () => {
  const valid = ajv.getSchema(`${schema.$id}#/$defs/${row.definition}`)(row.data);
  assert.equal(valid, row.valid);
});
for (const row of read('golden-manifests.json').vectors) test(`Go/Node digest: ${row.name}`, () => {
  const got = manifestDigest(row.manifest);
  assert.equal(got.canonical, row.canonical_payload); assert.equal(got.build_id, row.build_id);
  assert.equal(manifestDigest({ ...row.manifest, built_at: '2030-01-01T00:00:00.000Z' }).build_id, row.build_id);
  const changed = structuredClone(row.manifest); changed.artifacts[0].sha256 = '0'.repeat(64);
  assert.notEqual(manifestDigest(changed).build_id, row.build_id);
});
const semantics = read('semantic-cases.json');
test('artifact paths and duplicate paths', () => {
  for (const path of semantics.manifest_invalid_paths) assert.equal(validArtifactPath(path), false, path);
  const manifest = read('golden-manifests.json').vectors[0].manifest;
  manifest.artifacts.push(manifest.artifacts[0]); assert.throws(() => manifestDigest(manifest));
});
test('single site URL normalization', () => {
  for (const row of semantics.sites) assert.equal(normalizeSite(row.input), row.output);
  for (const site of semantics.invalid_sites) assert.throws(() => normalizeSite(site), site);
  for (const row of semantics.identify_sites) assert.equal(normalizeWebsite(row.input), row.output);
});
for (const row of semantics.time_cases) test(`monotonic lifetime: ${row.name}`, () => {
  const calculate = () => remainingMillis(semantics.server_time, semantics.challenge.expires_at, row.expires_in_ms, row.rtt_ms, row.existing_remaining_ms);
  if (row.expected_error) assert.throws(calculate); else assert.equal(calculate(), row.expected_remaining_ms);
});
test('strict JSON rejects duplicate names and invalid Unicode', () => {
  for (const raw of ['{"status":"recorded","status":"recorded"}', '{"status":"\\ud800"}', '{} {}']) assert.throws(() => parseStrictJSON(raw));
});
