#!/usr/bin/env node
/**
 * Provider release + secret rotation helper.
 *
 * Combines API-driven onboarding steps with the Go-based rotation scheduler so ops can
 * wire it into CI/CD or cron. Example usage:
 *
 *   node scripts/ops/provider-release.mjs register --token "$TOKEN" --payload examples/provider-openai.json
 *   node scripts/ops/provider-release.mjs validate --provider 7f4d... --suite full
 *   node scripts/ops/provider-release.mjs publish --provider 7f4d... --body publish.json
 *   node scripts/ops/provider-release.mjs rotate --provider 7f4d... --env staging
 *
 * The rotate command invokes the `/internal/providers/:id/rotate-secrets` admin API,
 * which calls the ProviderRegistryService.RotateProvider method and publishes metrics/audits.
 */

import { readFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";

const API_ROOT = process.env.POWERX_INTERNAL_API || "https://api.powerx.local/internal";

async function main() {
  const [command, ...rest] = process.argv.slice(2);
  if (!command || command === "--help" || command === "-h") {
    printHelp();
    process.exit(0);
  }
  const args = parseArgs(rest);
  switch (command) {
    case "register":
      await postJSON("/providers/register", await bodyFromFile(args.payload), args);
      break;
    case "validate":
      ensure(args.provider, "--provider is required");
      await postJSON(`/providers/${args.provider}/validate?suite=${args.suite || "full"}`, {}, args);
      break;
    case "publish":
      ensure(args.provider, "--provider is required");
      await postJSON(`/providers/${args.provider}/publish`, await bodyFromFile(args.body), args);
      break;
    case "rotate":
      await rotateSecrets(args);
      break;
    case "cron":
      await runCron(args);
      break;
    default:
      console.error(`Unknown command: ${command}`);
      printHelp();
      process.exit(1);
  }
}

async function rotateSecrets(args) {
  ensure(args.provider, "--provider is required");
  const env = args.env || "default";
  const body = { env };
  if (args.dryRun) {
    console.log(`[dry-run] would rotate secrets for ${args.provider} in ${env}`);
    return;
  }
  await postJSON(`/providers/${args.provider}/rotate-secrets`, body, args);
  // Kick the Go rotation scheduler once via make target if defined (no-op when unavailable).
  spawnSync("make", ["provider-rotation"], { stdio: "ignore" });
}

async function runCron(args) {
  const env = args.env || "staging";
  try {
    await postJSON("/ops/provider-rotation", { env }, args);
  } catch (err) {
    console.error("cron rotation failed:", err.message);
    process.exitCode = 1;
  }
}

async function postJSON(path, body, args) {
  const endpoint = new URL(path.replace(/^\/+/, "/"), API_ROOT);
  const res = await fetch(endpoint, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      ...(args.token ? { Authorization: `Bearer ${args.token}` } : {}),
    },
    body: JSON.stringify(body || {}),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`HTTP ${res.status} ${res.statusText}: ${text}`);
  }
  const json = await res.json().catch(() => ({}));
  console.log(JSON.stringify(json, null, 2));
  return json;
}

async function bodyFromFile(filePath) {
  if (!filePath) return {};
  const raw = await readFile(filePath, "utf-8");
  return JSON.parse(raw);
}

function parseArgs(argv) {
  const out = {};
  for (let i = 0; i < argv.length; i++) {
    const key = argv[i];
    if (!key.startsWith("--")) continue;
    const value = argv[i + 1]?.startsWith("--") || argv[i + 1] === undefined ? true : argv[++i];
    out[key.slice(2)] = value;
  }
  return out;
}

function ensure(value, message) {
  if (!value) {
    throw new Error(message);
  }
}

function printHelp() {
  console.log(`
Usage: provider-release <command> [options]
Commands:
  register --payload file.json --token xxx        Register provider draft via HTTP API
  validate --provider <uuid> [--suite full]       Trigger validation suite
  publish  --provider <uuid> --body publish.json  Publish provider rollout
  rotate   --provider <uuid> [--env default]      Invoke ProviderRegistry rotation endpoint
  cron     [--env staging]                        Fire-and-forget rotation for schedulers

Options:
  --token <token>             Bearer token for internal APIs
  --payload/--body <file>     JSON file containing request body
  --dryRun                    Skip network calls (logs actions only)
  --suite <name>              Validation suite (llm|vlm|tts|full)
`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
