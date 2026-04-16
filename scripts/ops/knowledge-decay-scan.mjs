#!/usr/bin/env node

import {mkdir, readFile, writeFile} from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import crypto from 'node:crypto';

const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..');
const defaultThresholdPath = path.join(repoRoot, 'backend', 'config', 'knowledge', 'decay_thresholds.yaml');
const defaultDraftPath = path.join(repoRoot, 'tmp', `knowledge-decay-scan-${Date.now()}.json`);
const defaultReportPath = path.join(repoRoot, 'backend', 'reports', '_state', 'knowledge-decay.json');
const defaultAggregatePath = path.join(repoRoot, 'reports', '_state', 'knowledge-update.json');

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
		console.info('📝 Dry-run result (no file written):');
		console.info(JSON.stringify(payload, null, 2));
		return;
	}

	await mkdir(path.dirname(args.output), {recursive: true});
	await writeFile(args.output, JSON.stringify(payload, null, 2));

	if (args.baseUrl) {
		const scanResp = await runRemoteScan(args, payload);
		console.info('✅ remote scan created tasks:', scanResp.taskIds.length);
		if (args.restoreFirst && scanResp.taskIds.length > 0) {
			await runRemoteRestore(args, scanResp.taskIds[0]);
			console.info('✅ remote restore completed (first task)');
		}
	}

	if (args.printReports) {
		await printLocalReports(args.reportPath, args.aggregatePath);
	}

	console.info(`✅ decay scan draft written to ${args.output}`);
}

function parseArgs(argv) {
	const args = {
		spaceId: '00000000-0000-0000-0000-000000000000',
		detected: 5,
		category: '',
		severity: '',
		thresholdsPath: defaultThresholdPath,
		output: defaultDraftPath,
		baseUrl: process.env.POWERX_BASE_URL || '',
		token: process.env.ADMIN_TOKEN || '',
		tenantUuid: process.env.TENANT_UUID || process.env.POWERX_TENANT_UUID || '',
		restoreFirst: true,
		printReports: true,
		reportPath: defaultReportPath,
		aggregatePath: defaultAggregatePath,
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
			case 'base-url':
				args.baseUrl = value;
				break;
			case 'token':
				args.token = value;
				break;
			case 'tenant-uuid':
				args.tenantUuid = value;
				break;
			case 'restore-first':
				args.restoreFirst = value !== '0' && value !== 'false';
				break;
			case 'print-reports':
				args.printReports = value !== '0' && value !== 'false';
				break;
			case 'report-path':
				args.reportPath = value;
				break;
			case 'aggregate-path':
				args.aggregatePath = value;
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
		request: {
			spaceId,
			detected: tasks.length,
			category: threshold.category,
			severity: threshold.severity,
			reason: threshold.reason,
		},
	};
}

async function runRemoteScan(args, payload) {
	if (!args.baseUrl) {
		throw new Error('缺少 --base-url 或 POWERX_BASE_URL');
	}
	if (!args.token) {
		throw new Error('缺少 --token 或 ADMIN_TOKEN');
	}
	if (!args.tenantUuid) {
		throw new Error('缺少 --tenant-uuid 或 TENANT_UUID/POWERX_TENANT_UUID');
	}
	const url = `${normalizeApiBase(args.baseUrl)}/knowledge/decay/tasks`;
	const resp = await fetch(url, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			Authorization: `Bearer ${args.token}`,
		},
		body: JSON.stringify(payload.request),
	});
	if (!resp.ok) {
		throw new Error(`remote scan failed: ${resp.status} ${await resp.text()}`);
	}
	const data = await resp.json();
	const tasks = data?.data?.tasks ?? [];
	return {taskIds: tasks.map((t) => t.uuid).filter(Boolean)};
}

async function runRemoteRestore(args, taskId) {
	const url = `${normalizeApiBase(args.baseUrl)}/knowledge/decay/restore`;
	const resp = await fetch(url, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			Authorization: `Bearer ${args.token}`,
		},
		body: JSON.stringify({
			taskId,
			falsePositive: true,
			notes: 'auto-restore for verification',
			reason: 'verification flow',
			approvedBy: 'ops-script',
		}),
	});
	if (!resp.ok) {
		throw new Error(`remote restore failed: ${resp.status} ${await resp.text()}`);
	}
	return resp.json();
}

function normalizeApiBase(raw) {
	const trimmed = String(raw || '').trim().replace(/\/$/, '');
	if (!trimmed) return 'http://127.0.0.1:8077/api/v1';
	if (trimmed.endsWith('/api/v1')) return trimmed;
	if (trimmed.endsWith('/api')) return `${trimmed}/v1`;
	if (trimmed.includes('/api/v1/')) return trimmed.replace(/\/$/, '');
	if (trimmed.includes('/api/')) return trimmed.replace(/\/$/, '');
	return `${trimmed}/api/v1`;
}

async function printLocalReports(reportPath, aggregatePath) {
	try {
		const raw = await readFile(reportPath, 'utf8');
		console.info('--- backend report: knowledge-decay.json ---');
		console.info(raw);
	} catch (err) {
		console.warn(`[warn] cannot read ${reportPath}: ${err.message}`);
	}
	try {
		const raw = await readFile(aggregatePath, 'utf8');
		console.info('--- aggregate report: knowledge-update.json ---');
		console.info(raw);
	} catch (err) {
		console.warn(`[warn] cannot read ${aggregatePath}: ${err.message}`);
	}
}

main().catch((err) => {
	console.error('❌ knowledge-decay-scan failed:', err.message);
	process.exit(1);
});
