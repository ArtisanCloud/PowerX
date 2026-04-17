#!/usr/bin/env node

/**
 * Generate a burndown snapshot for issues labeled `tenant-uuid-ga`.
 * Usage:
 *   GITHUB_TOKEN=xxx GITHUB_REPOSITORY=Org/Repo node scripts/ops/tenant-uuid-burndown.mjs
 * Optional env:
 *   LABEL_NAME   (default tenant-uuid-ga)
 *   OUTPUT       (default tmp/reports/tenant-uuid-burndown.json)
 */

import { mkdir, writeFile } from 'fs/promises';
import path from 'path';

const token = process.env.GITHUB_TOKEN || process.env.GH_TOKEN;
if (!token) {
  console.error('GITHUB_TOKEN 或 GH_TOKEN 必须设置以调用 GitHub GraphQL API');
  process.exit(1);
}

const repo = process.env.GITHUB_REPOSITORY || process.env.REPO;
if (!repo || !repo.includes('/')) {
  console.error('GITHUB_REPOSITORY（格式 owner/repo）不能为空');
  process.exit(1);
}
const [owner, name] = repo.split('/');
const label = process.env.LABEL_NAME || 'tenant-uuid-ga';
const output = process.env.OUTPUT || 'tmp/reports/tenant-uuid-burndown.json';

const gqlEndpoint = 'https://api.github.com/graphql';

async function fetchIssues(states) {
  const issues = [];
  let cursor = null;
  while (true) {
    const query = `
      query($owner:String!, $name:String!, $label:String!, $states:[IssueState!], $cursor:String) {
        repository(owner:$owner, name:$name) {
          issues(labels: [$label], states: $states, first: 100, after: $cursor) {
            nodes {
              number
              title
              url
              state
              createdAt
              closedAt
            }
            pageInfo {
              hasNextPage
              endCursor
            }
          }
        }
      }
    `;
    const body = JSON.stringify({
      query,
      variables: { owner, name, label, states, cursor }
    });
    const res = await fetch(gqlEndpoint, {
      method: 'POST',
      headers: {
        Authorization: `bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`GitHub GraphQL 请求失败：${res.status} ${text}`);
    }
    const payload = await res.json();
    if (payload.errors) {
      throw new Error(`GraphQL errors: ${JSON.stringify(payload.errors)}`);
    }
    const data = payload.data.repository.issues;
    issues.push(...data.nodes);
    if (!data.pageInfo.hasNextPage) {
      break;
    }
    cursor = data.pageInfo.endCursor;
  }
  return issues;
}

const openIssues = await fetchIssues(['OPEN']);
const closedIssues = await fetchIssues(['CLOSED']);
const now = Date.now();
const sevenDays = 7 * 24 * 60 * 60 * 1000;

const recentlyClosed = closedIssues
  .filter(issue => issue.closedAt && now - Date.parse(issue.closedAt) <= sevenDays)
  .map(issue => ({
    number: issue.number,
    title: issue.title,
    closedAt: issue.closedAt,
    url: issue.url
  }));

const report = {
  generatedAt: new Date().toISOString(),
  label,
  repository: repo,
  totals: {
    open: openIssues.length,
    closed: closedIssues.length,
    all: openIssues.length + closedIssues.length
  },
  openIssues: openIssues.map(issue => ({
    number: issue.number,
    title: issue.title,
    createdAt: issue.createdAt,
    url: issue.url
  })),
  recentlyClosed
};

await mkdir(path.dirname(output), { recursive: true });
await writeFile(output, JSON.stringify(report, null, 2));

console.info(`Burndown snapshot written to ${output}`);
console.info(`Open issues: ${openIssues.length}, closed issues: ${closedIssues.length}`);
