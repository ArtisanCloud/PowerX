# Localization Workflow Playbook

This guide walks documentation maintainers through the end-to-end translation workflow for keeping the PowerX docs in sync across zh-CN and en-US.

## Roles & Statuses

- **Placeholder**: Machine-translated scaffold created by the mirroring script. Banner warns readers and links back to zh-CN.
- **InReview**: Human editor is polishing the draft. Banner softens to “In Review”.
- **Approved**: Editorial review complete. Banner disappears and review guard passes.

Use these values in markdown frontmatter: `reviewStatus: Placeholder|InReview|Approved`.

## Daily Workflow

1. **Sync placeholders**
   ```sh
   pnpm run localization:sync
   ```
   - Mirrors every zh-CN `.md` file into `docs/en/**`.
   - Injects `partnerSlug`, default `reviewStatus: Placeholder`, and `/` slugs in the localization manifest.

2. **Translate & edit**
   - Update English content in-place.
   - Set `reviewStatus: InReview` while collaborating.
   - Switch to `reviewStatus: Approved` once the reviewer signs off.

3. **Spot-check parity**
   ```sh
   pnpm run localization:check
   ```
   - Reports missing English counterparts, placeholder counts, and partnerSlug mismatches.
   - Non-zero exit if any zh-CN page lacks an en-US mirror.

4. **Smoke-test defaults**
   ```sh
   node scripts/localization/assert-zh-default.mjs
   ```
   - Verifies `/` loads zh-CN copy, `<html lang="zh-CN">`, and unprefixed navigation links.

5. **Gate releases**
   ```sh
   pnpm run docs:release
   ```
   - Runs `localization:review-guard` before building.
   - Fails fast if any English file is missing `reviewStatus`, still `Placeholder`/`InReview`, or lacks a zh-CN source.

## Review Banner Reference

- Component: `docs/.vitepress/theme/components/ReviewBanner.vue`
- Automatically appears above docs when:
  - Locale is English **and**
  - `reviewStatus` is `Placeholder` or `InReview`
- Reads `partnerSlug` to link the visitor back to the zh-CN original.

## CI Recommendations

- Run `pnpm run localization:check` nightly to catch drift as early as possible.
- Trigger `pnpm run docs:release` in release pipelines to ensure no non-approved content ships.
