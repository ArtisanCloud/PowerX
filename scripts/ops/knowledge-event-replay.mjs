#!/usr/bin/env node

import {readFile} from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..');
const reportPath = path.join(repoRoot, 'backend', 'reports', '_state', 'knowledge-event.json');

async function main() {
  try {
    const raw = await readFile(reportPath, 'utf8');
    console.info('--- latest event hotfix report ---');
    console.info(raw);
  } catch (err) {
    console.error('no event report available:', err.message);
    process.exit(1);
  }
}

main();
