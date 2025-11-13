#!/usr/bin/env node

/**
 * Provider drill script
 * ----------------------
 * Simulates sudden usage spikes for a tenant/provider pair, polls the Cost/Quota
 * APIs, and optionally pings Grafana/PagerDuty endpoints to confirm alarms.
 *
 * Example:
 *   node scripts/qa/provider-drill.mjs \
 *     --tenant-id demo-tenant \
 *     --provider-id 2b92d17c-9d35-4c22-8a8d-24ddf9a6f1d3 \
 *     --env staging \
 *     --spike 1200 \
 *     --events 4 \
 *     --api-base https://api.powerx.local/internal \
 *     --token "$ADMIN_TOKEN" \
 *     --grafana-url https://grafana.powerx.local/d/drill \
 *     --pagerduty-url https://events.pagerduty.com/v2/enqueue \
 *     --output tmp/provider-drill.json
 */

import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";

const DEFAULT_API_BASE = "https://api.powerx.local/internal";
const DEFAULT_OUTPUT = "reports/provider-drill.json";
const DEFAULT_ALERT_TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (!args.tenantId) {
    console.error("Missing --tenant-id");
    process.exit(1);
  }
  const startedAt = Date.now();
  const context = {
    apiBase: args.apiBase || DEFAULT_API_BASE,
    token: args.token || "",
    verbose: args.verbose,
  };

  console.log(
    `[DRILL] tenant=${args.tenantId} provider=${
      args.providerId || "tenant"
    } env=${args.env}`
  );

  const initial = await fetchSnapshot(context, {
    tenantId: args.tenantId,
    env: args.env,
  });
  const baselineUsage = initial?.totals?.usage || 0;

  const spikeAmount = args.spike ?? 800;
  const events = buildUsageEvents(spikeAmount, args.events || 3);
  await reportUsage(context, {
    tenantId: args.tenantId,
    providerId: args.providerId,
    env: args.env,
    events,
  });
  console.log(
    `[DRILL] reported ${events.length} usage events totalling $${spikeAmount.toFixed(
      2
    )}`
  );

  const poll = await pollSnapshot(context, {
    tenantId: args.tenantId,
    env: args.env,
    targetIncrease: spikeAmount * 0.8,
    maxAttempts: args.pollAttempts || 6,
    intervalMs: args.pollInterval || 2000,
    baselineUsage,
  });

  if (!poll.metTarget) {
    console.warn(
      `[WARN] usage increase did not reflect within ${
        poll.attempts
      } attempts. Last snapshot usage=${poll.snapshot?.totals?.usage || 0}`
    );
  } else {
    console.log(
      `[DRILL] usage increased by ${poll.delta.toFixed(
        2
      )}, status=${poll.statusFlags.join(",") || "healthy"}`
    );
  }

  if (args.action) {
    await enforceAction(context, {
      tenantId: args.tenantId,
      providerId: args.providerId,
      env: args.env,
      action: args.action,
      reason: args.reason || "provider-drill",
      ticketId: args.ticketId || `DRILL-${Date.now()}`,
    });
    console.log(`[DRILL] enforcement '${args.action}' submitted.`);
  }

  if (args.grafanaUrl) {
    await pingGrafana(args.grafanaUrl);
  }
  const alertResult = await waitForAlert(context, {
    tenantId: args.tenantId,
    env: args.env,
    targetStates: args.alertTargets || ["anomaly", "enforcement_required"],
    timeoutMs: args.alertTimeoutMs || DEFAULT_ALERT_TIMEOUT_MS,
    pollInterval: args.alertPollInterval || 5000,
  });

  if (args.pagerdutyUrl) {
    await notifyPagerDuty(args.pagerdutyUrl, {
      tenantId: args.tenantId,
      providerId: args.providerId,
      delta: poll.delta,
      status: poll.statusFlags,
      routingKey: args.pagerdutyRoutingKey,
    }).catch((err) => {
      console.warn(`[WARN] PagerDuty notification failed: ${err.message}`);
    });
  }

  const finishedAt = Date.now();
  const summary = {
    tenantId: args.tenantId,
    providerId: args.providerId || null,
    env: args.env || "default",
    startedAt: new Date(startedAt).toISOString(),
    finishedAt: new Date(finishedAt).toISOString(),
    durationMs: finishedAt - startedAt,
    spikeAmount,
    events,
    initialSnapshot: initial,
    finalSnapshot: poll.snapshot,
    triggeredStatuses: poll.statusFlags,
    grafanaUrl: args.grafanaUrl || null,
    pagerdutyUrl: args.pagerdutyUrl || null,
    alertResult,
  };

  if (args.output !== false) {
    const outputFile = args.output || DEFAULT_OUTPUT;
    const dir = path.dirname(outputFile);
    if (dir && dir !== ".") {
      await fs.mkdir(dir, { recursive: true }).catch(() => {});
    }
    await fs.writeFile(outputFile, JSON.stringify(summary, null, 2));
    console.log(`[DRILL] summary saved to ${outputFile}`);
  }
}

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i++) {
    const raw = argv[i];
    if (!raw.startsWith("--")) continue;
    const [flag, value] = raw.includes("=")
      ? raw.slice(2).split(/=(.*)/, 2)
      : [raw.slice(2), argv[i + 1]];
    switch (flag) {
      case "tenant-id":
        args.tenantId = value;
        if (!raw.includes("=")) i++;
        break;
      case "provider-id":
        args.providerId = value;
        if (!raw.includes("=")) i++;
        break;
      case "env":
        args.env = value;
        if (!raw.includes("=")) i++;
        break;
      case "spike":
        args.spike = Number(value);
        if (!raw.includes("=")) i++;
        break;
      case "events":
        args.events = Number(value);
        if (!raw.includes("=")) i++;
        break;
      case "action":
        args.action = value;
        if (!raw.includes("=")) i++;
        break;
      case "reason":
        args.reason = value;
        if (!raw.includes("=")) i++;
        break;
      case "ticket-id":
        args.ticketId = value;
        if (!raw.includes("=")) i++;
        break;
      case "api-base":
        args.apiBase = value;
        if (!raw.includes("=")) i++;
        break;
      case "token":
        args.token = value;
        if (!raw.includes("=")) i++;
        break;
      case "grafana-url":
        args.grafanaUrl = value;
        if (!raw.includes("=")) i++;
        break;
      case "pagerduty-url":
        args.pagerdutyUrl = value;
        if (!raw.includes("=")) i++;
        break;
      case "pagerduty-routing-key":
        args.pagerdutyRoutingKey = value;
        if (!raw.includes("=")) i++;
        break;
      case "output":
        args.output = value;
        if (!raw.includes("=")) i++;
        break;
      case "poll-attempts":
        args.pollAttempts = Number(value);
        if (!raw.includes("=")) i++;
        break;
      case "poll-interval":
        args.pollInterval = Number(value);
        if (!raw.includes("=")) i++;
        break;
      case "verbose":
        args.verbose = true;
        break;
      case "alert-timeout":
        args.alertTimeoutMs = Number(value);
        if (!raw.includes("=")) i++;
        break;
      case "alert-poll-interval":
        args.alertPollInterval = Number(value);
        if (!raw.includes("=")) i++;
        break;
      case "alert-targets":
        args.alertTargets = value.split(",").map((item) => item.trim()).filter(Boolean);
        if (!raw.includes("=")) i++;
        break;
      default:
        console.warn(`Unknown flag: --${flag}`);
    }
  }
  return args;
}

async function fetchSnapshot(context, { tenantId, env }) {
  const url = withBase(context.apiBase, "/provider-quotas");
  const snapshot = await request(url, {
    method: "GET",
    token: context.token,
    params: { tenantId, env },
  });
  const totals = (snapshot?.quotas || []).reduce(
    (acc, quota) => {
      acc.limit += quota.limit || 0;
      acc.usage += quota.usage || 0;
      return acc;
    },
    { limit: 0, usage: 0 }
  );
  return { ...snapshot, totals };
}

async function reportUsage(context, payload) {
  const url = withBase(context.apiBase, "/provider-usage/report");
  return request(url, {
    method: "POST",
    token: context.token,
    body: {
      env: payload.env,
      tenantId: payload.tenantId,
      providerId: payload.providerId,
      events: payload.events.map((evt) => ({
        traceId: evt.traceId,
        tokens: evt.tokens,
        costUsd: evt.costUsd,
        timestamp: evt.timestamp,
      })),
    },
  });
}

async function enforceAction(context, payload) {
  const url = withBase(context.apiBase, "/provider-quotas/enforce");
  return request(url, {
    method: "POST",
    token: context.token,
    body: {
      env: payload.env,
      tenantId: payload.tenantId,
      providerId: payload.providerId,
      action: payload.action,
      reason: payload.reason,
      ticketId: payload.ticketId,
      requestedBy: "provider-drill",
    },
  });
}

async function pollSnapshot(context, options) {
  let snapshot = null;
  let attempts = 0;
  let metTarget = false;
  let delta = 0;
  let statusFlags = [];
  while (attempts < options.maxAttempts) {
    attempts++;
    snapshot = await fetchSnapshot(context, options);
    const increase =
      (snapshot?.totals?.usage || 0) - (options.baselineUsage || 0);
    delta = increase;
    statusFlags =
      snapshot?.quotas
        ?.filter((quota) => quota.status && quota.status !== "healthy")
        .map((quota) => quota.status) || [];
    if (increase >= options.targetIncrease || statusFlags.length > 0) {
      metTarget = true;
      break;
    }
    await delay(options.intervalMs);
  }
  return { snapshot, attempts, metTarget, delta, statusFlags };
}

function buildUsageEvents(totalCost, count) {
  const events = [];
  const step = totalCost / count;
  for (let i = 0; i < count; i++) {
    events.push({
      traceId: `drill-${i}-${Date.now()}`,
      tokens: Math.round(Math.random() * 5000),
      costUsd: step,
      timestamp: new Date(Date.now() - i * 1000).toISOString(),
    });
  }
  return events;
}

async function pingGrafana(url) {
  try {
    const res = await fetch(url, { method: "GET" });
    console.log(`[DRILL] Grafana probe ${url} -> ${res.status}`);
  } catch (err) {
    console.warn(`[WARN] Grafana probe failed for ${url}: ${err.message}`);
  }
}

async function notifyPagerDuty(url, payload) {
  const body = {
    routing_key: payload.routingKey || "provider-drill",
    event_action: "trigger",
    payload: {
      summary: `Provider drill for tenant ${payload.tenantId}`,
      source: "provider-drill",
      severity: "warning",
      custom_details: payload,
    },
  };
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(`PagerDuty responded with ${res.status}`);
  }
  console.log("[DRILL] PagerDuty notification sent.");
}

async function request(url, { method, token, body, params }) {
  const target = new URL(url);
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value === undefined || value === null || value === "") return;
      target.searchParams.set(key, value);
    });
  }
  const res = await fetch(target, {
    method,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`Request ${method} ${target} failed: ${res.status} ${text}`);
  }
  if (res.status === 204) return null;
  return res.json().catch(() => ({}));
}

function withBase(base, path) {
  const normalized = base.endsWith("/")
    ? base.slice(0, -1)
    : base;
  return `${normalized}${path}`;
}

const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function waitForAlert(context, { tenantId, env = "default", targetStates, timeoutMs, pollInterval }) {
  if (!targetStates || targetStates.length === 0) {
    return { awaited: false };
  }
  const started = Date.now();
  let attempts = 0;
  while (Date.now() - started < timeoutMs) {
    attempts++;
    const snapshot = await fetchSnapshot(context, { tenantId, env });
    const states =
      snapshot?.quotas
        ?.map((quota) => quota.status)
        .filter((status) => typeof status === "string" && status !== "healthy") || [];
    if (states.some((state) => targetStates.includes(state))) {
      return {
        awaited: true,
        succeeded: true,
        states,
        attempts,
        durationMs: Date.now() - started,
      };
    }
    await delay(pollInterval);
  }
  return {
    awaited: true,
    succeeded: false,
    attempts,
    durationMs: Date.now() - started,
  };
}

main().catch((err) => {
  console.error("[ERROR]", err.message);
  process.exit(1);
});
