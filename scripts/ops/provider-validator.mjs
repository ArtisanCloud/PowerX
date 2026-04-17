#!/usr/bin/env node

/**
 * Provider validation runner.
 *
 * Usage:
 *   node scripts/ops/provider-validator.mjs \
 *     --config configs/provider-openai.json \
 *     --provider-id 2b92d17c-9d35-4c22-8a8d-24ddf9a6f1d3 \
 *     --suite llm-regression \
 *     --output reports/provider-health.json \
 *     --api https://api.powerx.local/internal \
 *     --token "$ADMIN_TOKEN" \
 *     --var API_KEY=$OPENAI_KEY
 *
 * Config file format (JSON):
 * {
 *   "providerId": "optional-uuid",
 *   "env": {
 *     "API_KEY": "sk-***"
 *   },
 *   "tests": [
 *     {
 *       "name": "LLM echo",
 *       "modality": "llm",
 *       "request": {
 *         "url": "https://api.openai.com/v1/chat/completions",
 *         "method": "POST",
 *         "headers": {
 *           "Authorization": "Bearer ${API_KEY}",
 *           "Content-Type": "application/json"
 *         },
 *         "body": {
 *           "model": "gpt-4o-mini",
 *           "messages": [
 *             {"role":"system","content":"You are a health check probe."},
 *             {"role":"user","content":"Say hello in one short sentence"}
 *           ]
 *         },
 *         "timeout": 15000
 *       },
 *       "expect": {
 *         "status": 200,
 *         "bodyIncludes": "hello"
 *       }
 *     }
 *   ]
 * }
 */

import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";

const DEFAULT_API_BASE = "https://api.powerx.local/internal";
const DEFAULT_OUTPUT = "reports/provider-health.json";

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (!args.config) {
    console.error("Missing --config <file>");
    process.exit(1);
  }

  const config = await loadConfig(args.config);
  if (!config.tests || !Array.isArray(config.tests) || config.tests.length === 0) {
    console.error("Config must include at least one test");
    process.exit(1);
  }

  const providerId = args.providerId || config.providerId || "";
  const suite = args.suite || "full";
  const templateVars = {
    ...process.env,
    ...(config.env || {}),
    ...args.vars,
  };

  const results = [];
  let passed = 0;
  let failed = 0;

  for (const test of config.tests) {
    const result = await runTest(test, templateVars).catch((err) => ({
      name: test.name || test.modality,
      modality: test.modality || "unknown",
      success: false,
      error: err.message,
    }));
    if (result.success) {
      passed++;
    } else {
      failed++;
    }
    results.push(result);
    reportProgress(result);
  }

  const summary = {
    providerId,
    suite,
    generatedAt: new Date().toISOString(),
    stats: {
      total: results.length,
      passed,
      failed,
    },
    results,
  };

  await writeReport(args.output || DEFAULT_OUTPUT, summary);

  if (args.api && args.token && providerId) {
    await notifyRegistry({
      apiBase: args.api,
      token: args.token,
      providerId,
      suite,
      summary,
    }).catch((err) => {
      console.warn("Failed to update provider registry:", err.message);
    });
  }

  console.info(
    `Validation completed: ${passed}/${results.length} passed. Report saved to ${args.output || DEFAULT_OUTPUT}`
  );

  if (failed > 0) {
    process.exitCode = 2;
  }
}

function parseArgs(argv) {
  const args = {
    vars: {},
  };
  for (let i = 0; i < argv.length; i++) {
    const raw = argv[i];
    if (!raw.startsWith("--")) {
      continue;
    }
    const [flag, value] = raw.includes("=")
      ? raw.slice(2).split(/=(.*)/, 2)
      : [raw.slice(2), argv[i + 1]];

    switch (flag) {
      case "config":
        args.config = value;
        if (!raw.includes("=")) i++;
        break;
      case "provider-id":
        args.providerId = value;
        if (!raw.includes("=")) i++;
        break;
      case "suite":
        args.suite = value;
        if (!raw.includes("=")) i++;
        break;
      case "output":
        args.output = value;
        if (!raw.includes("=")) i++;
        break;
      case "api":
        args.api = value;
        if (!raw.includes("=")) i++;
        break;
      case "token":
        args.token = value;
        if (!raw.includes("=")) i++;
        break;
      case "var":
        {
          const assign = value || "";
          const [k, v] = assign.split("=", 2);
          if (k) {
            args.vars[k] = v ?? "";
          }
          if (!raw.includes("=")) i++;
        }
        break;
      default:
        console.warn(`Unknown flag: --${flag}`);
    }
  }
  args.api = args.api || DEFAULT_API_BASE;
  args.output = args.output || DEFAULT_OUTPUT;
  return args;
}

async function loadConfig(file) {
  const content = await fs.readFile(file, "utf8");
  try {
    return JSON.parse(content);
  } catch (err) {
    throw new Error(`Failed to parse config ${file}: ${err.message}`);
  }
}

async function runTest(test, templateVars) {
  const name = test.name || test.modality || "unnamed-test";
  const modality = test.modality || "unknown";
  const request = test.request || {};
  const method = (request.method || "GET").toUpperCase();
  const url = resolveTemplate(request.url || "", templateVars);
  if (!url) {
    throw new Error(`Test "${name}" is missing request.url`);
  }

  const headers = {};
  const headerEntries = Object.entries(request.headers || {});
  for (const [key, value] of headerEntries) {
    headers[key] = resolveTemplate(value, templateVars);
  }

  let body = request.body;
  if (body && typeof body === "object") {
    body = JSON.stringify(resolveObjectTemplates(body, templateVars));
    headers["Content-Type"] = headers["Content-Type"] || "application/json";
  } else if (typeof body === "string") {
    body = resolveTemplate(body, templateVars);
  }

  const controller = new AbortController();
  const timeoutMs = Number(request.timeout || 20000);
  const timeout = setTimeout(() => controller.abort(), timeoutMs);

  const start = performance.now();
  let response;
  let text = "";
  try {
    response = await fetch(url, {
      method,
      headers,
      body,
      signal: controller.signal,
    });
    text = await response.text();
  } finally {
    clearTimeout(timeout);
  }
  const latency = Math.round(performance.now() - start);

  const snippet = text.slice(0, 500);
  const success = evaluateExpectations(test.expect, response, text);

  return {
    name,
    modality,
    success,
    status: response.status,
    latencyMs: latency,
    endpoint: url,
    snippet,
    error: success ? undefined : buildErrorMessage(test.expect, response, snippet),
  };
}

function evaluateExpectations(expect, response, bodyText) {
  if (!expect) {
    return response.ok;
  }
  let ok = true;
  if (typeof expect.status === "number") {
    ok = ok && response.status === expect.status;
  } else {
    ok = ok && response.ok;
  }
  if (expect.bodyIncludes) {
    ok = ok && bodyText.toLowerCase().includes(String(expect.bodyIncludes).toLowerCase());
  }
  if (expect.bodyExcludes) {
    ok =
      ok &&
      !bodyText.toLowerCase().includes(String(expect.bodyExcludes).toLowerCase());
  }
  return ok;
}

function buildErrorMessage(expect, response, snippet) {
  if (!expect) {
    return `Unexpected status ${response.status}`;
  }
  if (typeof expect.status === "number" && response.status !== expect.status) {
    return `Expected status ${expect.status}, got ${response.status}`;
  }
  if (expect.bodyIncludes && !snippet.toLowerCase().includes(String(expect.bodyIncludes).toLowerCase())) {
    return `Body missing "${expect.bodyIncludes}"`;
  }
  if (expect.bodyExcludes && snippet.toLowerCase().includes(String(expect.bodyExcludes).toLowerCase())) {
    return `Body should not include "${expect.bodyExcludes}"`;
  }
  return "Expectation failed";
}

function resolveTemplate(str, vars) {
  if (typeof str !== "string") return str;
  return str.replace(/\$\{([A-Z0-9_]+)\}/gi, (_, key) => {
    return Object.prototype.hasOwnProperty.call(vars, key) ? vars[key] : "";
  });
}

function resolveObjectTemplates(obj, vars) {
  if (obj === null || typeof obj !== "object") {
    return obj;
  }
  if (Array.isArray(obj)) {
    return obj.map((item) => resolveObjectTemplates(item, vars));
  }
  const resolved = {};
  for (const [key, value] of Object.entries(obj)) {
    if (typeof value === "string") {
      resolved[key] = resolveTemplate(value, vars);
    } else {
      resolved[key] = resolveObjectTemplates(value, vars);
    }
  }
  return resolved;
}

function reportProgress(result) {
  const status = result.success ? "PASS" : "FAIL";
  console.info(
    `[${status}] ${result.name} (${result.modality}) - ${result.latencyMs ?? 0}ms`
  );
  if (!result.success && result.error) {
    console.info(`  ↳ ${result.error}`);
  }
}

async function writeReport(filePath, data) {
  const target = path.resolve(process.cwd(), filePath);
  const dir = path.dirname(target);
  await fs.mkdir(dir, { recursive: true });
  await fs.writeFile(target, JSON.stringify(data, null, 2), "utf8");
}

async function notifyRegistry({ apiBase, token, providerId, suite, summary }) {
  const url = `${apiBase.replace(/\/$/, "")}/providers/${providerId}/validate?suite=${encodeURIComponent(
    suite
  )}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ report: summary }),
  });
  if (!resp.ok) {
    throw new Error(`registry validate failed: ${resp.status}`);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
