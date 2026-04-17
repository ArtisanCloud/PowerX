#!/usr/bin/env node

/**
 * Routing Chaos Drill (SC-002 safe-mode validation).
 *
 * Simulates a tenant-scale outage by enabling safe-mode via the HTTP API,
 * confirms routing decisions fall back to secondary providers, and then
 * recovers by disabling safe-mode. Outputs a JSON report that can be
 * attached to chaos runbooks.
 *
 * Example:
 *   node scripts/qa/routing-chaos.mjs \
 *     --tenant demo-tenant \
 *     --token "$ADMIN_TOKEN" \
 *     --api http://127.0.0.1:8077/api \
 *     --task-type chat/general \
 *     --output tmp/routing-chaos-report.json
 */

import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";

const DEFAULT_API_BASE = "http://127.0.0.1:8077/api";
const DEFAULT_ENV = "default";
const DEFAULT_OUTPUT = "tmp/routing-chaos-report.json";
const DEFAULT_TASK_TYPE = "chat/general";
const DEFAULT_TTL_SECONDS = 300;

async function main() {
  const args = parseArgs(process.argv.slice(2));
  validateArgs(args);

  const context = await loadTaskContext(args);

  const summary = {
    tenant: args.tenant,
    env: args.env,
    generatedAt: new Date().toISOString(),
    apiBase: args.api,
    ttlSeconds: args.ttlSeconds,
    steps: {},
  };

  console.info(`[Chaos] Baseline routing check for tenant=${args.tenant}`);
  const baseline = await routeTask({
    apiBase: args.api,
    token: args.token,
    env: args.env,
    tenant: args.tenant,
    context,
  });
  summary.steps.baseline = baseline;

  console.info("[Chaos] Enabling safe-mode (simulated provider outage) ...");
  const enableState = await toggleSafeMode({
    apiBase: args.api,
    token: args.token,
    env: args.env,
    tenant: args.tenant,
    enabled: true,
    reason: args.reason,
    actor: args.actor,
    ttlSeconds: args.ttlSeconds,
  });
  summary.steps.enable = enableState;

  console.info("[Chaos] Routing during outage (expect fallback + safe-mode=true) ...");
  const degraded = await routeTask({
    apiBase: args.api,
    token: args.token,
    env: args.env,
    tenant: args.tenant,
    context,
  });
  summary.steps.degraded = degraded;

  console.info("[Chaos] Disabling safe-mode (recovery) ...");
  const disableState = await toggleSafeMode({
    apiBase: args.api,
    token: args.token,
    env: args.env,
    tenant: args.tenant,
    enabled: false,
    reason: "chaos-test-recovery",
    actor: args.actor,
    ttlSeconds: 0,
  });
  summary.steps.disable = disableState;

  const recovered = await routeTask({
    apiBase: args.api,
    token: args.token,
    env: args.env,
    tenant: args.tenant,
    context,
  });
  summary.steps.recovered = recovered;

  summary.verdict = buildVerdict(baseline, degraded, recovered);
  summary.nextSteps = [
    "Check Grafana: dashboards 'Model Hub Overview' & 'Safe Mode Monitor' should show spike in agent.routing.safe_mode_active and fallback counters.",
    "Verify PagerDuty / alert channel received safe-mode notification within the TTL window.",
    "Confirm Redis (or cache) entry for safe-mode key cleared after recovery (if applicable).",
  ];

  await writeReport(args.output, summary);
  console.info(
    `[Chaos] Completed. Report saved to ${args.output}. Verdict: ${
      summary.verdict.ok ? "PASS" : "FAIL"
    }`,
  );

  if (!summary.verdict.ok) {
    process.exitCode = 2;
  }
}

function parseArgs(argv) {
  const args = {
    api: DEFAULT_API_BASE,
    env: DEFAULT_ENV,
    output: DEFAULT_OUTPUT,
    taskType: DEFAULT_TASK_TYPE,
    ttlSeconds: DEFAULT_TTL_SECONDS,
    reason: "chaos-test",
    actor: "routing-chaos-cli",
  };
  for (let i = 0; i < argv.length; i++) {
    const raw = argv[i];
    if (!raw.startsWith("--")) continue;
    const [flag, value] = raw.includes("=")
      ? raw.slice(2).split(/=(.*)/, 2)
      : [raw.slice(2), argv[i + 1]];
    const consume = () => {
      if (!raw.includes("=")) i++;
    };
    switch (flag) {
      case "tenant":
        args.tenant = value;
        consume();
        break;
      case "token":
        args.token = value;
        consume();
        break;
      case "api":
        args.api = value || DEFAULT_API_BASE;
        consume();
        break;
      case "env":
        args.env = value || DEFAULT_ENV;
        consume();
        break;
      case "output":
        args.output = value || DEFAULT_OUTPUT;
        consume();
        break;
      case "task-type":
        args.taskType = value || DEFAULT_TASK_TYPE;
        consume();
        break;
      case "context":
        args.contextFile = value;
        consume();
        break;
      case "ttl":
        args.ttlSeconds = Math.max(30, Number(value) || DEFAULT_TTL_SECONDS);
        consume();
        break;
      case "reason":
        args.reason = value || args.reason;
        consume();
        break;
      case "actor":
        args.actor = value || args.actor;
        consume();
        break;
      default:
        console.warn(`Unknown flag --${flag}`);
    }
  }
  return args;
}

function validateArgs(args) {
  const missing = [];
  if (!args.tenant) missing.push("--tenant");
  if (!args.token) missing.push("--token");
  if (missing.length) {
    console.error(`Missing required flags: ${missing.join(", ")}`);
    process.exit(1);
  }
}

async function loadTaskContext(args) {
  if (args.contextFile) {
    const content = await fs.readFile(path.resolve(args.contextFile), "utf8");
    return JSON.parse(content);
  }
  return {
    taskType: args.taskType,
  };
}

async function routeTask({ apiBase, token, env, tenant, context }) {
  const resp = await fetch(`${trimSlash(apiBase)}/internal/model-routing/route`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({
      env,
      tenantId: tenant,
      taskContext: context,
    }),
  });
  const body = await resp.json().catch(() => ({}));
  if (!resp.ok) {
    const error = (body?.message || body?.error || "unknown error").toString();
    return {
      ok: false,
      error: `route failed (${resp.status}) ${error}`,
      response: body,
    };
  }
  const data = body?.data || {};
  return {
    ok: true,
    provider: data.primaryProviderId,
    fallbackUsed: !!data.fallbackUsed,
    safeMode: !!data.safeMode,
    traceId: data.traceId,
    policyVersion: data.policyVersion,
    raw: data,
  };
}

async function toggleSafeMode({ apiBase, token, env, tenant, enabled, reason, actor, ttlSeconds }) {
  const resp = await fetch(`${trimSlash(apiBase)}/internal/model-routing/safe-mode`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({
      env,
      tenantScope: tenant,
      enabled,
      reason,
      actor,
      ttlSeconds,
    }),
  });
  const body = await resp.json().catch(() => ({}));
  if (!resp.ok) {
    const error = (body?.message || body?.error || "unknown error").toString();
    throw new Error(`safe-mode toggle failed (${resp.status}): ${error}`);
  }
  return body?.data?.state || body?.data || body;
}

function buildVerdict(baseline, degraded, recovered) {
  const verdict = {
    ok: true,
    checks: {},
  };

  verdict.checks.initialHit = baseline.ok && !baseline.fallbackUsed;
  verdict.checks.safeModeFallback = degraded.ok && degraded.safeMode && degraded.fallbackUsed;
  verdict.checks.recovery = recovered.ok && !recovered.safeMode && !recovered.fallbackUsed;

  verdict.ok = verdict.checks.initialHit && verdict.checks.safeModeFallback && verdict.checks.recovery;
  return verdict;
}

async function writeReport(file, payload) {
  const absolute = path.resolve(file);
  await fs.mkdir(path.dirname(absolute), { recursive: true });
  await fs.writeFile(absolute, JSON.stringify(payload, null, 2), "utf8");
}

function trimSlash(url) {
  return url.replace(/\/$/, "");
}

if (import.meta.main) {
  main().catch((err) => {
    console.error("[routing-chaos]", err);
    process.exit(1);
  });
}

