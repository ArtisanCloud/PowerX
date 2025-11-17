#!/usr/bin/env node

import {mkdir, readFile, writeFile} from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import crypto from 'node:crypto';

const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..');
const defaultThresholdPath = path.join(repoRoot, 'configs', 'knowledge', 'decay_thresholds.yaml');
const defaultReportPath = path.join(repoRoot, 'tmp', 'knowledge-decay-report.json');

async function main() {
	const args = parseArgs(process.argv.slice(2));
	const thresholds = await loadThresholds(args.thresholdsPath);
	const selected = selectThreshold(thresholds, args.category, args.severity);
	const payload = buildReport({
		spaceId: args.spaceId,
		detected: args.detected,
		threshold: selected,
	});

	if (args.dryRun) {
		console.log('📝 Dry-run result (no file written):');
		console.log(JSON.stringify(payload, null, 2));
		return;
	}

	await mkdir(path.dirname(args.output), {recursive: true});
	await writeFile(args.output, JSON.stringify(payload, null, 2));
	console.log(`✅ Decay scan report written to ${args.output}`);
}

function parseArgs(argv) {
	const args = {
		spaceId: '00000000-0000-0000-0000-000000000000',
		detected: 5,
		category: '',
		severity: '',
		thresholdsPath: defaultThresholdPath,
		output: defaultReportPath,
		dryRun: false,
	};
	for (const token of argv) {
		if (token === '--dry-run') {
			args.dryRun = true;
			continue;
		}
		const [rawKey, value] = token.split('=');
		if (!rawKey || value === undefined) continue;
		const key = rawKey.replace(/^--/, '');
		switch (key) {
			case 'space':
				args.spaceId = value;
				break;
			case 'detected':
				args.detected = Math.max(1, Number.parseInt(value, 10));
				break;
			case 'category':
				args.category = value;
				break;
			case 'severity':
				args.severity = value;
				break;
			case 'thresholds':
				args.thresholdsPath = value;
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

async function loadThresholds(filePath) {
	const raw = await readFile(filePath, 'utf8');
	return JSON.parse(raw);
}

function selectThreshold(config, category, severity) {
	const list = config?.thresholds ?? [];
	if (list.length === 0) {
		return {
			category: category || 'coverage',
			severity: severity || 'p2',
			maxAgeHours: 72,
			slaHours: 168,
			reason: '默认阈值',
		};
	}
	return (
		list.find((item) => item.category === category && category) ||
		list.find((item) => item.severity === severity && severity) ||
		list[0]
	);
}

function buildReport({spaceId, detected, threshold}) {
	const now = new Date();
	const slaHours = Number(threshold.slaHours ?? 24 * 7);
	const slaDue = new Date(now.getTime() + slaHours * 60 * 60 * 1000);
	const tasks = Array.from({length: detected}, (_, idx) => ({
		taskId: crypto.randomUUID(),
		spaceId,
		sequence: idx + 1,
		category: threshold.category,
		severity: threshold.severity,
		reason: threshold.reason,
		detectedAt: now.toISOString(),
		slaDueAt: slaDue.toISOString(),
	}));
	return {
		metadata: {
			spaceId,
			generatedAt: now.toISOString(),
			threshold: threshold.category,
			severity: threshold.severity,
		},
		tasks,
		metrics: {
			"knowledge.decay.detected": tasks.length,
			"knowledge.gap.backlog": tasks.length,
			"knowledge.decay.sla_hours": slaHours,
		},
	};
}

main().catch((err) => {
	console.error('❌ knowledge-decay-scan failed:', err.message);
	process.exit(1);
});
