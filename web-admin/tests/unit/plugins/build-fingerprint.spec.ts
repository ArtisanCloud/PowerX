// @vitest-environment node
import { expect, it } from "vitest";
import { readFile, mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { createHash } from "node:crypto";
import { digest, generate } from "../../../scripts/build-fingerprint.mjs";

it("实际发布生成器与共同摘要样例一致", async () => {
  const fixtures = JSON.parse(await readFile(new URL("../../../../contracts/deployment-fingerprint/v1/golden-manifests.json", import.meta.url), "utf8"));
  for (const row of fixtures.vectors) expect(digest(row.manifest)).toBe(row.build_id);
});
it("清单覆盖最终公开资源，修改产物后摘要随之变化", async () => {
  const root = await mkdtemp(join(tmpdir(), "powerx-fingerprint-assets-"));
  try {
    await mkdir(join(root, "_nuxt"));
    await writeFile(join(root, "_nuxt/app.js"), "final js");
    const a = await generate(root, "1.0.0", "revision", "2026-10-23T00:00:00Z");
    expect(a.artifacts[0].sha256).toBe(createHash("sha256").update("final js").digest("hex"));
    expect(JSON.parse(await readFile(join(root, ".well-known/powerx.json"), "utf8")).build_id).toBe(a.build_id);
    await writeFile(join(root, "_nuxt/app.js"), "modified js");
    expect((await generate(root, "1.0.0", "revision", "2026-10-23T00:00:00Z")).build_id).not.toBe(a.build_id);
  } finally { await rm(root, { recursive: true, force: true }); }
});
