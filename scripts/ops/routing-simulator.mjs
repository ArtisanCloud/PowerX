#!/usr/bin/env node

/**
 * Routing simulator: replays task contexts against the routing API and
 * reports provider selections / fallback usage for a given tenant scope.
 *
 * Example:
 *   node scripts/ops/routing-simulator.mjs \
 *     --tenant demo-tenant \
 *     --scenario critical_tasks \
 *     --policy rc-2025-11-10 \
 *     --token "$ADMIN_TOKEN"
 */

import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";

const DEFAULT_API_BASE = "https://api.powerx.local/internal";
const DEFAULT_SCENARIO_FILE = "scripts/ops/scenarios/routing/default.json";
const DEFAULT_OUTPUT = "reports/routing-simulator-report.json";
const DEFAULT_ENV = "default";
const DEFAULT_TIMEOUT = 10000; // ms
const DEFAULT_HIT_THRESHOLD = 0.9;
const DEFAULT_FALLBACK_THRESHOLD = 0.95;
const DEFAULT_SAFE_MODE_WINDOW_SECONDS = 300; // 5 minutes

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const scenarios = await loadScenarios(args.scenarioFile);
  if (args.list) {
    listScenarios(scenarios);
    return;
  }

  validateRequiredArgs(args);

  const scenario = scenarios[args.scenario];
  if (!scenario) {
    console.error(`Scenario "${args.scenario}" not found in ${args.scenarioFile}`);
    process.exit(1);
  }

  console.info(
    `Running routing simulator for tenant=${args.tenant} scenario=${args.scenario} env=${args.env}`
  );
  const summary = await runScenario({
    apiBase: args.api,
    token: args.token,
    env: args.env,
    tenant: args.tenant,
    policyLabel: args.policy,
    scenarioName: args.scenario,
    scenario,
    timeout: args.timeout,
    iterations: args.iterations,
    thresholds: {
      hit: args.hitThreshold,
      fallback: args.fallbackThreshold,
      safeModeWindowMs: args.safeModeWindowSeconds * 1000,
    },
    requireSafeMode: args.requireSafeMode,
  });

  await writeReport(args.output, summary);
  console.info(
    `Simulation complete: ${summary.metrics.total} tasks replayed, fallbackRate=${summary.metrics.fallbackRate.toFixed(
      2
    )}. Report saved to ${args.output}`
  );
  logSloResults(summary.slo);
  if (!summary.slo.overall) {
    process.exitCode = 2;
  }
}

function parseArgs(argv) {
  const args = {
    api: DEFAULT_API_BASE,
    scenarioFile: DEFAULT_SCENARIO_FILE,
    output: DEFAULT_OUTPUT,
    env: DEFAULT_ENV,
    timeout: DEFAULT_TIMEOUT,
    iterations: 1,
    hitThreshold: DEFAULT_HIT_THRESHOLD,
    fallbackThreshold: DEFAULT_FALLBACK_THRESHOLD,
    safeModeWindowSeconds: DEFAULT_SAFE_MODE_WINDOW_SECONDS,
    requireSafeMode: false,
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
      case "scenario":
        args.scenario = value;
        consume();
        break;
      case "scenario-file":
        args.scenarioFile = value;
        consume();
        break;
      case "tenant":
        args.tenant = value;
        consume();
        break;
      case "policy":
        args.policy = value;
        consume();
        break;
      case "env":
        args.env = value;
        consume();
        break;
      case "api":
        args.api = value;
        consume();
        break;
      case "token":
        args.token = value;
        consume();
        break;
      case "output":
        args.output = value;
        consume();
        break;
      case "timeout":
        args.timeout = Number(value) || DEFAULT_TIMEOUT;
        consume();
        break;
      case "iterations":
        args.iterations = Math.max(1, Number(value) || 1);
        consume();
        break;
      case "hit-threshold":
        args.hitThreshold = clamp01(Number(value), DEFAULT_HIT_THRESHOLD);
        consume();
        break;
      case "fallback-threshold":
        args.fallbackThreshold = clamp01(Number(value), DEFAULT_FALLBACK_THRESHOLD);
        consume();
        break;
      case "safe-mode-window":
        args.safeModeWindowSeconds = Math.max(1, Number(value) || DEFAULT_SAFE_MODE_WINDOW_SECONDS);
        consume();
        break;
      case "require-safe-mode":
        args.requireSafeMode = true;
        break;
      case "list":
        args.list = true;
        break;
      default:
        console.warn(`Unknown flag: --${flag}`);
    }
  }
  args.scenarioFile = path.resolve(args.scenarioFile);
  args.output = path.resolve(args.output);
  return args;
}

function validateRequiredArgs(args) {
  const missing = [];
  if (!args.tenant) missing.push("--tenant");
  if (!args.scenario) missing.push("--scenario");
  if (!args.token) missing.push("--token");
  if (missing.length > 0) {
    console.error(`Missing required flags: ${missing.join(", ")}`);
    process.exit(1);
  }
}

async function loadScenarios(file) {
  const content = await fs.readFile(file, "utf8");
  const parsed = JSON.parse(content);
  const scenarios = parsed.scenarios || {};
  return scenarios;
}

function listScenarios(scenarios) {
  console.info("Available scenarios:");
  for (const [name, spec] of Object.entries(scenarios)) {
    console.info(
      `  - ${name}${spec?.description ? `: ${spec.description}` : ""} (tasks: ${
        Array.isArray(spec?.tasks) ? spec.tasks.length : 0
      })`
    );
  }
}

async function runScenario(options) {
  const {
    apiBase,
    token,
    env,
    tenant,
    policyLabel,
    scenarioName,
    scenario,
    timeout,
    iterations,
    thresholds,
    requireSafeMode,
  } = options;

  const tasks = Array.isArray(scenario.tasks) ? scenario.tasks : [];
  if (tasks.length === 0) {
    throw new Error(`Scenario "${scenarioName}" has no tasks defined`);
  }

  const runStartedAt = Date.now();
  let safeModeFirstMs = null;
  const results = [];
  for (let iteration = 0; iteration < iterations; iteration++) {
    for (const task of tasks) {
      const result = await replayTask({
        apiBase,
        token,
        env,
        tenant,
        task,
        timeout,
      });
      result.iteration = iteration + 1;
      results.push(result);
      logTaskResult(result);
      if (result.safeMode && safeModeFirstMs === null) {
        safeModeFirstMs = Date.now() - runStartedAt;
      }
    }
  }

  const metrics = summarizeResults(results);
  const slo = evaluateSlo({
    metrics,
    thresholds,
    safeModeFirstMs,
    requireSafeMode,
  });
  return {
    policy: policyLabel || "active",
    tenant,
    env,
    scenario: scenarioName,
    generatedAt: new Date().toISOString(),
    iterations,
    metrics,
    thresholds,
    safeModeFirstTriggerMs: safeModeFirstMs,
    safeModeFirstTriggerSeconds: safeModeFirstMs != null ? safeModeFirstMs / 1000 : null,
    requireSafeMode,
    slo,
    results,
  };
}

async function replayTask({ apiBase, token, env, tenant, task, timeout }) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeout);

  const payload = {
    env,
    tenantId: tenant,
    taskContext: task.context || {},
  };

  const start = performance.now();
  try {
    const resp = await fetch(`${apiBase.replace(/\/$/, "")}/model-routing/route`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(payload),
      signal: controller.signal,
    });
    const elapsedMs = performance.now() - start;
    if (!resp.ok) {
      const text = await resp.text();
      throw new Error(`HTTP ${resp.status}: ${text}`);
    }
    const body = await resp.json();
    const data = body?.data || {};
    const expectation = buildExpectation(task.expect);
    const success = expectation ? expectation(data) : true;
    return {
      name: task.name || task.context?.taskType || "task",
      context: task.context || {},
      decision: data,
      latencyMs: Number(elapsedMs.toFixed(2)),
      fallbackUsed: Boolean(data.fallbackUsed),
      safeMode: Boolean(data.safeMode),
      matchedRule: data.matchedRule || "",
      traceId: data.traceId,
      success,
      error: success ? undefined : "Expectation mismatch",
      timestamp: new Date().toISOString(),
    };
  } catch (err) {
    return {
      name: task.name || task.context?.taskType || "task",
      context: task.context || {},
      decision: null,
      latencyMs: Number((performance.now() - start).toFixed(2)),
      fallbackUsed: false,
      safeMode: false,
      matchedRule: "",
      success: false,
      error: err.name === "AbortError" ? "Request timed out" : err.message,
      timestamp: new Date().toISOString(),
    };
  } finally {
    clearTimeout(timer);
  }
}

function buildExpectation(expectConfig = {}) {
  const expectedProvider = expectConfig.preferredProvider;
  const allowFallback = Boolean(expectConfig.allowFallback);
  if (!expectedProvider && !allowFallback) {
    return null;
  }
  return (decision) => {
    if (!decision) return false;
    if (expectedProvider && decision.primaryProviderId !== expectedProvider) {
      return false;
    }
    if (allowFallback === false && decision.fallbackUsed) {
      return false;
    }
    return true;
  };
}

function summarizeResults(results) {
  const total = results.length;
  const failures = results.filter((r) => !r.success).length;
  const fallbackCount = results.filter((r) => r.fallbackUsed).length;
  const safeModeCount = results.filter((r) => r.safeMode).length;
  const fallbackSuccessCount = results.filter((r) => r.fallbackUsed && r.success).length;
  const latencyAvg =
    results.reduce((sum, r) => sum + (r.latencyMs || 0), 0) / Math.max(total, 1);

  const providerBreakdown = {};
  for (const r of results) {
    const provider = r.decision?.primaryProviderId || "n/a";
    providerBreakdown[provider] = (providerBreakdown[provider] || 0) + 1;
  }

  return {
    total,
    failures,
    fallbackRate: total > 0 ? fallbackCount / total : 0,
    safeModeRate: total > 0 ? safeModeCount / total : 0,
    fallbackSuccessRate: fallbackCount > 0 ? fallbackSuccessCount / fallbackCount : 1,
    hitRate: total > 0 ? (total - fallbackCount) / total : 0,
    avgLatencyMs: Number(latencyAvg.toFixed(2)),
    providerBreakdown,
  };
}

function logTaskResult(result) {
  const status = result.success ? "PASS" : "FAIL";
  const provider = result.decision?.primaryProviderId || "n/a";
  console.info(
    `[${status}] ${result.name} -> ${provider} (${result.latencyMs}ms)${
      result.fallbackUsed ? " [fallback]" : ""
    }${result.safeMode ? " [safe-mode]" : ""}${result.error ? ` :: ${result.error}` : ""}`
  );
}

async function writeReport(outputPath, summary) {
  await fs.mkdir(path.dirname(outputPath), { recursive: true });
  await fs.writeFile(outputPath, JSON.stringify(summary, null, 2), "utf8");
}

function evaluateSlo({ metrics, thresholds, safeModeFirstMs, requireSafeMode }) {
  const hitPass = metrics.hitRate >= thresholds.hit;
  const fallbackPass = metrics.fallbackSuccessRate >= thresholds.fallback;
  let safeModePass = true;
  if (requireSafeMode) {
    safeModePass =
      safeModeFirstMs !== null && safeModeFirstMs <= thresholds.safeModeWindowMs;
  }
  return {
    hitPass,
    fallbackPass,
    safeModePass,
    overall: hitPass && fallbackPass && safeModePass,
    details: {
      hitRate: metrics.hitRate,
      fallbackSuccessRate: metrics.fallbackSuccessRate,
      safeModeFirstTriggerMs: safeModeFirstMs,
      safeModeRequirement: requireSafeMode,
      thresholds,
    },
  };
}

function logSloResults(slo) {
  console.info(
    `[SLO] hitRate ok=${slo.hitPass} fallbackSuccess ok=${slo.fallbackPass} safeMode ok=${slo.safeModePass} overall=${slo.overall}`,
  );
  if (!slo.overall) {
    console.warn("[SLO] details:", JSON.stringify(slo.details, null, 2));
  }
}

function clamp01(value, fallback) {
  if (Number.isNaN(value)) return fallback;
  return Math.min(1, Math.max(0, value));
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
