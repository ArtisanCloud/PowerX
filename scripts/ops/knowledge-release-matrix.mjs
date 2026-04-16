#!/usr/bin/env node

import {readFile, writeFile} from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..');
const defaultMatrix = path.join(repoRoot, 'backend', 'config', 'knowledge', 'tenant_release_matrix.yaml');
const defaultReleaseReport = path.join(repoRoot, 'backend', 'reports', '_state', 'knowledge-release.json');
const defaultAggregateReport = path.join(repoRoot, 'reports', '_state', 'knowledge-update.json');

async function main() {
	const args = parseArgs(process.argv.slice(2));
	const matrix = await loadMatrix(args.matrix);
	lintMatrix(matrix);
	printSummary(matrix);
	if (args.export) {
		await writeFile(args.export, JSON.stringify(matrix, null, 2));
		console.info(`✅ Matrix exported to ${args.export}`);
	}

	if (args.baseUrl) {
		assertFlagEnabled('PX_TENANT_RELEASE_MATRIX');
		const policyId = await upsertPolicy(args, matrix);
		if (args.publishVersion) {
			assertFlagEnabled('PX_KNOWLEDGE_GRAY_RELEASE');
			const published = await publishRelease(args, policyId, args.publishVersion);
			console.info(`✅ publish ok: releaseId=${published.releaseId} batchToken=${published.batchToken}`);
			if (args.autoPromote) {
				let token = published.batchToken;
				for (;;) {
					const promoted = await promoteRelease(args, policyId, args.publishVersion, token, []);
					console.info(`✅ promote: state=${promoted.state} coverage=${promoted.tenantCoverage}`);
					if (!promoted.nextBatchToken) break;
					token = promoted.nextBatchToken;
				}
			}
		}
		if (args.rollbackReason) {
			assertFlagEnabled('PX_KNOWLEDGE_GRAY_RELEASE');
			await rollbackRelease(args, policyId, args.rollbackVersion || args.publishVersion || '', args.rollbackReason);
			console.info('✅ rollback requested');
		}
		if (args.printStatus && (args.publishVersion || args.rollbackVersion)) {
			const version = args.rollbackVersion || args.publishVersion;
			const status = await fetchStatus(args, policyId, version);
			console.info('--- release status ---');
			console.info(JSON.stringify(status, null, 2));
		}
	}

	if (args.printReports) {
		await printLocalReports(args.releaseReportPath, args.aggregateReportPath);
	}
}

function parseArgs(argv) {
	const args = {
		matrix: defaultMatrix,
		export: '',
		baseUrl: process.env.POWERX_BASE_URL || '',
		token: process.env.ADMIN_TOKEN || '',
		tenantUuid: process.env.TENANT_UUID || process.env.POWERX_TENANT_UUID || '',
		publishVersion: '',
		rollbackVersion: '',
		rollbackReason: '',
		autoPromote: false,
		printStatus: true,
		printReports: true,
		releaseReportPath: defaultReleaseReport,
		aggregateReportPath: defaultAggregateReport,
	};
	for (const token of argv) {
		const [rawKey, value] = token.split('=');
		const key = rawKey?.replace(/^--/, '');
		if (!key) continue;
		switch (key) {
			case 'matrix':
				args.matrix = value || args.matrix;
				break;
			case 'export':
				args.export = value || '';
				break;
			case 'base-url':
				args.baseUrl = value || '';
				break;
			case 'token':
				args.token = value || '';
				break;
			case 'tenant-uuid':
				args.tenantUuid = value || '';
				break;
			case 'publish':
				args.publishVersion = value || '';
				break;
			case 'auto-promote':
				args.autoPromote = value !== '0' && value !== 'false';
				break;
			case 'rollback-version':
				args.rollbackVersion = value || '';
				break;
			case 'rollback':
				args.rollbackReason = value || '';
				break;
			case 'print-status':
				args.printStatus = value !== '0' && value !== 'false';
				break;
			case 'print-reports':
				args.printReports = value !== '0' && value !== 'false';
				break;
			case 'release-report':
				args.releaseReportPath = value || args.releaseReportPath;
				break;
			case 'aggregate-report':
				args.aggregateReportPath = value || args.aggregateReportPath;
				break;
			case 'help':
				printHelp();
				process.exit(0);
			default:
				break;
		}
	}
	return args;
}

function printHelp() {
	console.info(`knowledge-release-matrix

Usage:
  node scripts/ops/knowledge-release-matrix.mjs [--matrix=path] [--export=path]
  node scripts/ops/knowledge-release-matrix.mjs --base-url=$POWERX_BASE_URL --token=$ADMIN_TOKEN --tenant-uuid=$TENANT_UUID --publish=ver-2025.02 --auto-promote=true

Options:
  --matrix   指定 tenant_release_matrix 文件（默认 backend/config/knowledge/tenant_release_matrix.yaml）
  --export   导出校验通过的 JSON（可供 CLI 或 Pipeline 使用）
  --base-url 远端 API Base URL（默认 POWERX_BASE_URL）
  --token   Admin Token（默认 ADMIN_TOKEN）
  --tenant-uuid 租户 UUID（默认 TENANT_UUID/POWERX_TENANT_UUID，用于鉴权上下文）
  --publish 发布指定版本（会先 upsert policy）
  --auto-promote  自动推进所有批次（默认 false）
  --rollback-version 回滚指定版本（默认取 --publish 的版本）
  --rollback 回滚原因（非空则触发 rollback）
  --print-reports 打印本地 reports/_state 快照（默认 true）
`);
}

async function loadMatrix(filePath) {
	const raw = await readFile(filePath, 'utf8');
	try {
		return JSON.parse(raw);
	} catch (err) {
		throw new Error(`无法解析 ${filePath}（当前仓库约定为 JSON 内容，尽管扩展名为 .yaml）: ${err.message}`);
	}
}

function lintMatrix(matrix) {
	const errors = [];
	if (!matrix || typeof matrix !== 'object') {
		errors.push('matrix 文件内容为空');
	}
	if (!matrix?.matrixVersion) {
		errors.push('缺少 matrixVersion');
	}
	if (!Array.isArray(matrix?.batches) || matrix.batches.length === 0) {
		errors.push('缺少 batches 定义');
	}
	const seenTenants = new Map();
	(matrix?.batches || []).forEach((batch, index) => {
		const tenants = batch?.tenants || [];
		if (!Array.isArray(tenants) || tenants.length === 0) {
			errors.push(`批次 ${batch?.name || index} 缺少 tenants`);
		}
		for (const tenant of tenants) {
			if (!tenant) continue;
			const lower = tenant.toLowerCase();
			if (seenTenants.has(lower)) {
				errors.push(`租户 ${tenant} 在多个批次中重复（首次出现在 ${seenTenants.get(lower)}）`);
			} else {
				seenTenants.set(lower, batch?.name || `batch-${index}`);
			}
		}
	});
	if (errors.length) {
		errors.forEach((msg) => console.error(`❌ ${msg}`));
		process.exit(1);
	}
}

function printSummary(matrix) {
	const tenants = new Set();
	(matrix?.batches || []).forEach((batch) => {
		(batch?.tenants || []).forEach((tenant) => tenants.add(tenant));
	});
	console.info('📋 Release Matrix Summary');
	console.info(`- Matrix Version: ${matrix.matrixVersion}`);
	console.info(`- Pilot Tenants: ${(matrix.pilotTenants || []).join(', ') || 'N/A'}`);
	console.info(`- Batch Count: ${matrix.batches?.length || 0}`);
	console.info(`- Tenant Coverage: ${tenants.size}`);
	console.info('- Guardrails:');
	Object.entries(matrix.guardrails || {}).forEach(([key, value]) => {
		console.info(`  • ${key}: ${value}`);
	});
}

function assertFlagEnabled(flag) {
	const value = String(process.env[flag] || '').trim().toLowerCase();
	if (!value) return;
	if (['0', 'false', 'disabled', 'off', 'no'].includes(value)) {
		throw new Error(`feature flag disabled: ${flag}`);
	}
}

async function upsertPolicy(args, matrix) {
	if (!args.baseUrl) throw new Error('missing --base-url / POWERX_BASE_URL');
	if (!args.token) throw new Error('missing --token / ADMIN_TOKEN');
	if (!args.tenantUuid) throw new Error('missing --tenant-uuid / TENANT_UUID');
	const url = `${normalizeApiBase(args.baseUrl)}/knowledge/release/policies`;
	const resp = await fetch(url, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			Authorization: `Bearer ${args.token}`,
		},
		body: JSON.stringify({
			matrixVersion: matrix.matrixVersion,
			pilotTenants: matrix.pilotTenants || [],
			batches: matrix.batches || [],
			guardrails: matrix.guardrails || {},
			approvedBy: matrix.approvedBy || 'ops-script',
			createdBy: matrix.createdBy || 'ops-script',
		}),
	});
	if (!resp.ok) throw new Error(`upsert policy failed: ${resp.status} ${await resp.text()}`);
	const data = await resp.json();
	return String(data?.data?.policyId || '');
}

async function publishRelease(args, policyId, versionId) {
	const url = `${normalizeApiBase(args.baseUrl)}/knowledge/release/publish`;
	const resp = await fetch(url, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			Authorization: `Bearer ${args.token}`,
		},
		body: JSON.stringify({policyId, versionId, requestedBy: 'ops-script'}),
	});
	if (!resp.ok) throw new Error(`publish failed: ${resp.status} ${await resp.text()}`);
	const data = await resp.json();
	return data?.data || {};
}

async function promoteRelease(args, policyId, versionId, batchToken, alerts) {
	const url = `${normalizeApiBase(args.baseUrl)}/knowledge/release/promote`;
	const resp = await fetch(url, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			Authorization: `Bearer ${args.token}`,
		},
		body: JSON.stringify({policyId, versionId, batchToken, alerts: alerts || [], requestedBy: 'ops-script'}),
	});
	if (!resp.ok) throw new Error(`promote failed: ${resp.status} ${await resp.text()}`);
	const data = await resp.json();
	return data?.data || {};
}

async function rollbackRelease(args, policyId, versionId, reason) {
	if (!versionId) throw new Error('missing versionId for rollback');
	const url = `${normalizeApiBase(args.baseUrl)}/knowledge/release/rollback`;
	const resp = await fetch(url, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			Authorization: `Bearer ${args.token}`,
		},
		body: JSON.stringify({policyId, versionId, reason, requestedBy: 'ops-script'}),
	});
	if (!resp.ok) throw new Error(`rollback failed: ${resp.status} ${await resp.text()}`);
	return resp.json();
}

async function fetchStatus(args, policyId, versionId) {
	const params = new URLSearchParams({policyId, versionId});
	const url = `${normalizeApiBase(args.baseUrl)}/knowledge/release/status?${params.toString()}`;
	const resp = await fetch(url, {
		method: 'GET',
		headers: {
			Authorization: `Bearer ${args.token}`,
		},
	});
	if (!resp.ok) throw new Error(`status failed: ${resp.status} ${await resp.text()}`);
	const data = await resp.json();
	return data?.data?.status || null;
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

async function printLocalReports(releaseReportPath, aggregateReportPath) {
	try {
		const raw = await readFile(releaseReportPath, 'utf8');
		console.info('--- backend report: knowledge-release.json ---');
		console.info(raw);
	} catch (err) {
		console.warn(`[warn] cannot read ${releaseReportPath}: ${err.message}`);
	}
	try {
		const raw = await readFile(aggregateReportPath, 'utf8');
		console.info('--- aggregate report: knowledge-update.json ---');
		console.info(raw);
	} catch (err) {
		console.warn(`[warn] cannot read ${aggregateReportPath}: ${err.message}`);
	}
}

main().catch((err) => {
	console.error('❌ knowledge-release-matrix failed:', err.message);
	process.exit(1);
});
