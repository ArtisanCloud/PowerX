#!/usr/bin/env node

import {readFile, writeFile, mkdir} from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..');
const sourcesPath = path.join(repoRoot, 'backend', 'config', 'knowledge', 'delta_sources.yaml');

async function main() {
	const args = parseArgs(process.argv.slice(2));
	const config = await loadJSON(sourcesPath);
	const sources = config?.sources ?? [];
	const sourceName = args.source || (sources[0]?.name ?? 'default');
	const source = sources.find((item) => item.name === sourceName);
	if (!source) {
		throw new Error(`未在 ${sourcesPath} 找到源 ${sourceName}`);
	}
	const job = {
		spaceId: args.space || '00000000-0000-0000-0000-000000000000',
		source: source.name,
		packageUri: args.package || source.endpoint,
		generatedAt: new Date().toISOString(),
	};
	const outPath = args.output || path.join(repoRoot, 'tmp', `delta-start-${Date.now()}.json`);
	await mkdir(path.dirname(outPath), {recursive: true});
	await writeFile(outPath, JSON.stringify(job, null, 2));
	console.log(`✅ delta job draft written to ${outPath}`);
}

function parseArgs(argv) {
	const args = {};
	for (const token of argv) {
		const [key, value] = token.split('=');
		if (!key || !value) continue;
		switch (key.replace(/^--/, '')) {
			case 'space':
				args.space = value;
				break;
			case 'source':
				args.source = value;
				break;
			case 'package':
				args.package = value;
				break;
			case 'output':
				args.output = value;
				break;
			default:
				break;
		}
	}
	return args;
}

async function loadJSON(filePath) {
	const raw = await readFile(filePath, 'utf8');
	return JSON.parse(raw);
}

function cryptoRandom() {
	return 'job-' + Math.random().toString(16).slice(2, 10);
}

main().catch((err) => {
	console.error('❌ delta job script failed:', err.message);
	process.exit(1);
});
