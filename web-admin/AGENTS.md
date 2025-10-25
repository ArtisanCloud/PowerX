# Repository Guidelines

## Project Structure & Module Organization
PowerX Admin is a Nuxt 4 application. Core UI work happens in `app/`: `app/pages/` defines routes, `app/components/` holds shared widgets, `app/composables/` wraps reusable logic, and `app/stores/` contains Pinia state. Middleware and server handlers sit in `server/` and `app/server/`, static assets belong in `public/`, and long-form specs live in `docs/`. Keep locale strings in `i18n/locales/*.json` and update `scripts/check-refactor.sh` whenever agent-critical files move.

## Build, Test, and Development Commands
Install dependencies with `npm install` after every pull. Use `npm run dev` for the hot-reloading workspace, `npm run build` for the Nitro production bundle, and `npm run preview` to validate the bundle locally. `npm run generate` produces a static export for demos or documentation drops. Run `bash scripts/check-refactor.sh` before shipping agent-layer changes to confirm expected file layout.

## Coding Style & Naming Conventions
Stick to TypeScript in `<script setup lang="ts">` blocks and two-space indentation. Components use PascalCase filenames (e.g., `ChatInterface.vue`), composables follow `useX.ts`, and Pinia stores reside in `app/stores` with `useXStore`. Keep schema types in `app/types/` and align naming with locale keys. Tailwind v4 utilities are the primary styling tool; prefer semantic class groupings over inline styles. Enforce lint rules via `npx eslint . --ext .ts,.vue --fix`, backed by `eslint.config.mjs` and the Nuxt ESLint preset.

## Testing Guidelines
Automated tests are minimal today, so smoke-test authentication, dashboards, and agent chat flows via `npm run dev` before merging. When adding coverage, place specs in `tests/` or nearby `__tests__/` folders and name files `*.spec.ts`. Favor Vitest with Nuxt Test Utils; run suites with `npx vitest --run` once the dependency is added. Always rerun `scripts/check-refactor.sh` after touching agent messaging utilities.

## Commit & Pull Request Guidelines
Follow the existing history by using conventional prefixes (`feat(auth):`, `update(api):`, `fix:`) and an imperative summary. Keep commits focused—avoid mixing formatting with feature work. Pull requests must describe user impact, list key modules touched, link to the tracking issue, and attach screenshots or GIFs for UI updates. Call out locale updates or config changes explicitly and tag reviewers responsible for the affected area.
