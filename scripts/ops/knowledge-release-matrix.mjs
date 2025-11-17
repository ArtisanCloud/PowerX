#!/usr/bin/env node

import {readFile, writeFile} from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..');
const defaultMatrix = path.join(repoRoot, 'configs', 'knowledge', 'tenant_release_matrix.yaml');

async function main() {
	const args = parseArgs(process.argv.slice(2));
	const matrix = await loadMatrix(args.matrix);
	lintMatrix(matrix);
	printSummary(matrix);
	if (args.export) {
		await writeFile(args.export, JSON.stringify(matrix, null, 2));
		console.log(`✅ Matrix exported to ${args.export}`);
	}
}

function parseArgs(argv) {
	const args = {
		matrix: defaultMatrix,
		export: '',
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
	console.log(`knowledge-release-matrix

Usage:
  node scripts/ops/knowledge-release-matrix.mjs [--matrix=path] [--export=path]

Options:
  --matrix   指定 tenant_release_matrix 文件（默认 configs/knowledge/tenant_release_matrix.yaml）
  --export   导出校验通过的 JSON（可供 CLI 或 Pipeline 使用）
`);
}

async function loadMatrix(filePath) {
	const raw = await readFile(filePath, 'utf8');
	try {
		return JSON.parse(raw);
	} catch (err) {
		throw new Error(`无法解析 ${filePath}，请确保使用 JSON 格式: ${err.message}`);
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
	console.log('📋 Release Matrix Summary');
	console.log(`- Matrix Version: ${matrix.matrixVersion}`);
	console.log(`- Pilot Tenants: ${(matrix.pilotTenants || []).join(', ') || 'N/A'}`);
	console.log(`- Batch Count: ${matrix.batches?.length || 0}`);
	console.log(`- Tenant Coverage: ${tenants.size}`);
	console.log('- Guardrails:');
	Object.entries(matrix.guardrails || {}).forEach(([key, value]) => {
		console.log(`  • ${key}: ${value}`);
	});
}

main().catch((err) => {
	console.error('❌ knowledge-release-matrix failed:', err.message);
	process.exit(1);
});
