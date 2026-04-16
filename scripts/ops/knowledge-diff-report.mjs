#!/usr/bin/env node

import {readFile} from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..');
const deltaReport = path.join(repoRoot, 'backend', 'reports', '_state', 'knowledge-delta.json');
const aggregateReport = path.join(repoRoot, 'reports', '_state', 'knowledge-update.json');

async function main() {
	const delta = await loadIfExists(deltaReport);
	const aggregate = await loadIfExists(aggregateReport);
	console.info('--- Knowledge Delta Snapshot ---');
	console.info(JSON.stringify(delta ?? {message: 'no delta snapshot'}, null, 2));
	console.info('\n--- Knowledge Update Aggregate ---');
	console.info(JSON.stringify(aggregate ?? {message: 'aggregate missing'}, null, 2));
}

async function loadIfExists(filePath) {
	try {
		const raw = await readFile(filePath, 'utf8');
		return JSON.parse(raw);
	} catch (err) {
		return null;
	}
}

main().catch((err) => {
	console.error('❌ delta diff report failed:', err.message);
	process.exit(1);
});
