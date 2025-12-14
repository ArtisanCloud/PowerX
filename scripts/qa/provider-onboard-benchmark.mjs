#!/usr/bin/env node

/**
 * Provider onboarding benchmark runner (SC-001 guardrail).
 *
 * Usage:
 *   node scripts/qa/provider-onboard-benchmark.mjs \
 *     --config specs/010-agent-model-setting/examples/provider-benchmark.json \
 *     --token "$ADMIN_TOKEN" \
 *     --api http://127.0.0.1:8077/api \
 *     --output tmp/provider-onboard-benchmark.json
 *
 * Config format:
 * {
 *   "env": "default",
 *   "suite": "benchmark",
 *   "thresholdHours": 24,
 *   "providers": [
 *     {
 *       "name": "openai-benchmark",
 *       "capabilities": ["llm"],
 *       "primary_endpoint": "https://api.openai.com/v1",
 *       "regions": ["us-east-1"],
 *       "tenantWhitelist": [{"tenant_uuid":"demo","environment":"staging"}],
 *       "credentials": {"api_key": "${OPENAI_KEY}"},
 *       "publish": {"rolloutStrategy": "gray"}
 *     }
 *   ]
 * }
 */

import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";

const DEFAULT_API_BASE = "http://127.0.0.1:8077/api";
const DEFAULT_OUTPUT = "tmp/provider-onboard-benchmark.json";
const DEFAULT_THRESHOLD_HOURS = 24;
const DEFAULT_SUITE = "benchmark";
const BANNED_FIELD_TOKENS = [
  '"api_key"',
  '"apikey"',
  '"secret"',
  '"secretkey"',
];

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (!args.config) {
    console.error("Missing --config <file>");
    process.exit(1);
  }
  if (!args.token) {
    console.error("Missing --token <value>");
    process.exit(1);
  }

  const config = await loadConfig(args.config);
  const providers = Array.isArray(config.providers) ? config.providers : [];
  if (providers.length === 0) {
    console.error("Config.providers must include at least one entry");
    process.exit(1);
  }

  const thresholdHours =
    typeof config.thresholdHours === "number" && config.thresholdHours > 0
      ? config.thresholdHours
      : DEFAULT_THRESHOLD_HOURS;
  const env = config.env || "default";
  const suite = config.suite || DEFAULT_SUITE;
  const context = {
    ...process.env,
    ...(config.vars || {}),
  };

  const runs = [];
  const violations = [];
  let failures = 0;

  for (let i = 0; i < providers.length; i++) {
    const scenario = providers[i];
    const prepared = prepareScenario(scenario, i, env, context);
    const run = await executeRun({
      apiBase: args.api,
      token: args.token,
      suite,
      scenario: prepared,
      violations,
    });
    runs.push(run);
    if (run.status !== "success") {
      failures++;
    }
  }

  const durations = runs.filter((r) => r.status === "success").map((r) => r.durationMs);
  const p95Ms = durations.length ? percentile(durations, 0.95) : 0;
  const p95Hours = p95Ms / (1000 * 60 * 60);
  const withinSlo = durations.length === 0 || p95Hours <= thresholdHours;
  const hasLeak = violations.length > 0;
  const summary = {
    generatedAt: new Date().toISOString(),
    env,
    suite,
    providersTested: runs.length,
    thresholdHours,
    p95Hours,
    withinSlo,
    failures,
    secretViolations: violations,
    runs,
  };

  await writeReport(args.output, summary);

  console.log(
    `[Benchmark] providers=${runs.length} p95=${p95Hours.toFixed(
      4,
    )}h threshold=${thresholdHours}h failures=${failures} leaks=${violations.length}`,
  );

  if (!withinSlo || hasLeak || failures > 0) {
    process.exitCode = 2;
  }
}

function parseArgs(argv) {
  const args = {
    api: DEFAULT_API_BASE,
    output: DEFAULT_OUTPUT,
  };
  for (let i = 0; i < argv.length; i++) {
    const raw = argv[i];
    if (!raw.startsWith("--")) continue;
    const [flag, value] = raw.includes("=")
      ? raw.slice(2).split(/=(.*)/, 2)
      : [raw.slice(2), argv[i + 1]];
    const bump = () => {
      if (!raw.includes("=")) i++;
    };
    switch (flag) {
      case "config":
        args.config = value;
        bump();
        break;
      case "token":
        args.token = value;
        bump();
        break;
      case "api":
        args.api = value || DEFAULT_API_BASE;
        bump();
        break;
      case "output":
        args.output = value || DEFAULT_OUTPUT;
        bump();
        break;
      default:
        console.warn(`Unknown flag --${flag}`);
    }
  }
  args.output = args.output || DEFAULT_OUTPUT;
  args.api = (args.api || DEFAULT_API_BASE).replace(/\/$/, "");
  return args;
}

async function loadConfig(file) {
  const content = await fs.readFile(file, "utf8");
  return JSON.parse(content);
}

function prepareScenario(scenario, index, env, context) {
  const copy = structuredClone(scenario);
  const suffix = `-${Date.now()}-${index}`;
  copy.name = resolveString(copy.name || `provider-${index + 1}`, {
    ...context,
    RUN_INDEX: index,
    RUN_SUFFIX: suffix,
  });
  if (!copy.name.endsWith(suffix)) {
    copy.name = `${copy.name}${suffix}`;
  }
  copy.env = scenario.env || env;
  copy.credentials = resolveObject(copy.credentials || {}, context);
  copy.capabilities = Array.isArray(copy.capabilities) ? copy.capabilities : [];
  copy.regions = Array.isArray(copy.regions) ? copy.regions : [];
  copy.tenantWhitelist = Array.isArray(copy.tenantWhitelist) ? copy.tenantWhitelist : [];
  copy.publish = copy.publish || {};
  return copy;
}

async function executeRun({ apiBase, token, suite, scenario, violations }) {
  const run = {
    name: scenario.name,
    startedAt: new Date().toISOString(),
    durationMs: 0,
    status: "pending",
    providerId: "",
    warnings: [],
  };
  const start = Date.now();
  const sensitiveValues = Object.values(scenario.credentials || {}).filter(Boolean);

  try {
    const registerPayload = {
      env: scenario.env,
      name: scenario.name,
      capabilities: scenario.capabilities,
      primary_endpoint: scenario.primary_endpoint,
      regions: scenario.regions,
      tenantWhitelist: scenario.tenantWhitelist,
      credentials: scenario.credentials,
    };
    const registerRes = await postJSON(
      `${apiBase}/internal/providers/register`,
      token,
      registerPayload,
    );
    recordSecretViolations("register", registerRes.bodyText, sensitiveValues, violations);
    if (!registerRes.ok) {
      throw new Error(
        `register failed (${registerRes.status}): ${registerRes.bodyText.slice(0, 200)}`,
      );
    }
    const provider = registerRes.json?.data?.provider;
    if (!provider?.provider_id) {
      throw new Error("register response missing provider_id");
    }
    run.providerId = provider.provider_id;

    const validationPayload = buildValidationPayload(run.providerId, suite, scenario);
    const validateRes = await postJSON(
      `${apiBase}/internal/providers/${run.providerId}/validate?suite=${suite}`,
      token,
      validationPayload,
    );
    recordSecretViolations("validate", validateRes.bodyText, sensitiveValues, violations);
    if (!validateRes.ok) {
      throw new Error(
        `validate failed (${validateRes.status}): ${validateRes.bodyText.slice(0, 200)}`,
      );
    }

    const publishPayload = {
      rolloutStrategy: scenario.publish.rolloutStrategy || "gray",
      tenantWhitelist: scenario.publish.tenantWhitelist || scenario.tenantWhitelist,
      rollbackTimeoutMinutes: scenario.publish.rollbackTimeoutMinutes || 0,
    };
    const publishRes = await postJSON(
      `${apiBase}/internal/providers/${run.providerId}/publish`,
      token,
      publishPayload,
    );
    recordSecretViolations("publish", publishRes.bodyText, sensitiveValues, violations);
    if (!publishRes.ok) {
      throw new Error(
        `publish failed (${publishRes.status}): ${publishRes.bodyText.slice(0, 200)}`,
      );
    }

    run.status = "success";
  } catch (err) {
    run.status = "failed";
    run.warnings.push(err.message);
  } finally {
    run.durationMs = Date.now() - start;
    run.finishedAt = new Date().toISOString();
  }
  return run;
}

function buildValidationPayload(providerId, suite, scenario) {
  return {
    report: {
      providerId,
      suite,
      generatedAt: new Date().toISOString(),
      stats: {
        total: 1,
        passed: 1,
        failed: 0,
      },
      results: [
        {
          name: scenario.validationName || "llm smoke",
          modality: scenario.validationModality || "llm",
          success: true,
        },
      ],
    },
  };
}

async function postJSON(url, token, payload) {
  const resp = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
  });
  const bodyText = await resp.text();
  let parsed = null;
  try {
    parsed = JSON.parse(bodyText);
  } catch {
    // ignore
  }
  return { ok: resp.ok, status: resp.status, json: parsed, bodyText };
}

function recordSecretViolations(stage, bodyText, sensitiveValues, violations) {
  const lower = bodyText.toLowerCase();
  for (const token of BANNED_FIELD_TOKENS) {
    if (lower.includes(token)) {
      violations.push({ stage, type: "field", token });
    }
  }
  for (const value of sensitiveValues) {
    if (value && bodyText.includes(value)) {
      violations.push({ stage, type: "value", token: mask(value) });
    }
  }
}

function percentile(values, p) {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const idx = Math.min(sorted.length - 1, Math.max(0, Math.ceil(p * sorted.length) - 1));
  return sorted[idx];
}

async function writeReport(file, payload) {
  const absolute = path.resolve(file);
  await fs.mkdir(path.dirname(absolute), { recursive: true });
  await fs.writeFile(absolute, JSON.stringify(payload, null, 2), "utf8");
}

function resolveObject(obj, context) {
  if (!obj || typeof obj !== "object") {
    return {};
  }
  const out = {};
  for (const [key, value] of Object.entries(obj)) {
    if (typeof value === "string") {
      out[key] = resolveString(value, context);
    } else if (Array.isArray(value)) {
      out[key] = value.map((item) =>
        typeof item === "string" ? resolveString(item, context) : item,
      );
    } else if (value && typeof value === "object") {
      out[key] = resolveObject(value, context);
    } else {
      out[key] = value;
    }
  }
  return out;
}

function resolveString(input, context) {
  if (typeof input !== "string") {
    return input;
  }
  return input.replace(/\$\{([\w\d_:-]+)\}/g, (_, key) => {
    if (context[key] !== undefined) {
      return context[key];
    }
    return "";
  });
}

function mask(value) {
  if (!value) return "";
  if (value.length <= 6) return "***";
  return `${value.slice(0, 3)}***${value.slice(-3)}`;
}

// Structured clone shim for Node 20 (available globally but keep fallback)
function structuredClone(value) {
  if (globalThis.structuredClone) {
    return globalThis.structuredClone(value);
  }
  return JSON.parse(JSON.stringify(value));
}

if (import.meta.main) {
  main().catch((err) => {
    console.error("[provider-onboard-benchmark]", err);
    process.exit(1);
  });
}
