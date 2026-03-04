#!/usr/bin/env node

import {readFile, writeFile, mkdir} from 'node:fs/promises';
import {existsSync} from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import {fileURLToPath} from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..', '..');
const configPath = path.join(repoRoot, 'backend', 'config', 'knowledge', 'feedback_playbook.yaml');
const metricsPath = path.join(repoRoot, 'backend', 'reports', '_state', 'knowledge-feedback.json');
const aggregatePath = path.join(repoRoot, 'reports', '_state', 'knowledge-update.json');
const auditPath = path.join(repoRoot, 'backend', 'reports', '_state', 'knowledge-feedback-audit.json');
const ledgerPath = path.join(repoRoot, 'backend', 'reports', '_state', 'knowledge-feedback-ledger.json');

async function main() {
	const config = await loadConfig();
	const metrics = await loadMetrics();
	const summary = evaluate(metrics, config);
	await persistAggregate(summary);
	await writeAudit(summary, config);
	console.log('✅ feedback-loop summary updated');
}

async function loadConfig() {
	try {
		const raw = await readFile(configPath, 'utf8');
		return parseYaml(raw);
	} catch (err) {
		throw new Error(`无法读取 ${configPath}: ${err.message}`);
	}
}

async function loadMetrics() {
	if (!existsSync(metricsPath)) {
		return {
			loopTimeHours: 0,
			fixAccuracyPct: 0,
			autoRatePct: 0,
			backlog: 0,
			recordedAt: new Date().toISOString(),
		};
	}
	const raw = await readFile(metricsPath, 'utf8');
	try {
		return JSON.parse(raw);
	} catch (err) {
		throw new Error(`无法解析 ${metricsPath}: ${err.message}`);
	}
}

async function loadLedger() {
	if (!existsSync(ledgerPath)) {
		return null;
	}
	try {
		return JSON.parse(await readFile(ledgerPath, 'utf8'));
	} catch (err) {
		throw new Error(`无法解析 ${ledgerPath}: ${err.message}`);
	}
}

function evaluate(metrics, config) {
	const defaults = config?.defaults ?? {};
	const summary = {
		loopTimeHours: metrics.loopTimeHours ?? 0,
		loopSLAHours: defaults.sla_hours ?? 24,
		fixAccuracyPct: metrics.fixAccuracyPct ?? 0,
		minFixAccuracyPct: defaults.min_fix_accuracy_pct ?? 25,
		autoRatePct: metrics.autoRatePct ?? 0,
		minAutoRatePct: defaults.min_auto_rate_pct ?? 50,
		backlog: metrics.backlog ?? 0,
		backlogSoftLimit: defaults.backlog_soft_limit ?? 40,
		recordedAt: metrics.recordedAt ?? new Date().toISOString(),
	};
	summary.loopWithinSLA = summary.loopTimeHours <= summary.loopSLAHours;
	summary.fixAccuracyMet = summary.fixAccuracyPct >= summary.minFixAccuracyPct;
	summary.autoRateMet = summary.autoRatePct >= summary.minAutoRatePct;
	summary.backlogWithinLimit = summary.backlog <= summary.backlogSoftLimit;
	return summary;
}

async function persistAggregate(summary) {
	const aggregate = existsSync(aggregatePath)
		? JSON.parse(await readFile(aggregatePath, 'utf8'))
		: {};
	aggregate.feedback = summary;
	await mkdir(path.dirname(aggregatePath), {recursive: true});
	await writeFile(aggregatePath, JSON.stringify(aggregate, null, 2));
}

async function writeAudit(summary, config) {
	const ledger = await loadLedger();
	const auditPayload = {
		timestamp: new Date().toISOString(),
		type: 'knowledge.feedback.loop',
		loopWithinSLA: summary.loopWithinSLA,
		fixAccuracyMet: summary.fixAccuracyMet,
		autoRateMet: summary.autoRateMet,
		backlogWithinLimit: summary.backlogWithinLimit,
		backlog: summary.backlog,
		config: config?.defaults ?? {},
		auditLedger: ledger ?? undefined,
	};
	await mkdir(path.dirname(auditPath), {recursive: true});
	await writeFile(auditPath, JSON.stringify(auditPayload, null, 2));
}

function parseYaml(source) {
	const root = {};
	const stack = [{indent: -1, node: root}];
	const lines = source.split(/\r?\n/);
	for (const rawLine of lines) {
		if (!rawLine.trim() || rawLine.trim().startsWith('#')) {
			continue;
		}
		const indent = rawLine.match(/^\s*/)[0].length;
		let line = rawLine.trim();
		if (line.startsWith('- ')) {
			throw new Error('feedback_playbook.yaml 暂不支持数组语法');
		}
		const idx = line.indexOf(':');
		if (idx === -1) {
			continue;
		}
		const key = line.slice(0, idx).trim();
		let value = line.slice(idx + 1).trim();
		while (stack.length && indent <= stack[stack.length - 1].indent) {
			stack.pop();
		}
		const parent = stack[stack.length - 1].node;
		if (!value) {
			parent[key] = {};
			stack.push({indent, node: parent[key]});
			continue;
		}
		parent[key] = parseScalar(value);
	}
	return root;
}

function parseScalar(value) {
	if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
		return value.slice(1, -1);
	}
	if (!Number.isNaN(Number(value))) {
		return Number(value);
	}
	if (value === 'true' || value === 'false') {
		return value === 'true';
	}
	return value;
}

main().catch((err) => {
	console.error('❌ feedback loop validation failed:', err.message);
	process.exit(1);
});
