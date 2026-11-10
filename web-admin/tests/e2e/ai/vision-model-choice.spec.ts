import { test, expect, type Page } from '@playwright/test';

async function fixture(page: Page, empty = false) {
  const tenant = '00000000-0000-4000-8000-000000000001';
  await page.addInitScript(tenant => {
    const expiresAt = Date.now() + 3600_000;
    const token = `header.${btoa(JSON.stringify({ exp: Math.floor(expiresAt / 1000), tid: tenant }))}.fixture`;
    localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'vision-choice-proof', revision: 1, accessToken: token, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: tenant }));
    localStorage.setItem('welcome_guide_completed', 'true'); localStorage.setItem('px_locale', 'zh');
  }, tenant);
  const image = { env: 'dev', modality: 'image', provider: 'openai', model: 'gpt-image-2', defaults: { size: '1536x1024', quality: 'high', format: 'webp' } };
  let vision: any = empty ? null : { env: 'dev', modality: 'vlm', provider: 'openai', model: 'tts-1', defaults: { max_tokens: 2048 } };
  const originalImage = JSON.stringify(image), saved: any[] = [], inference: string[] = [], errors: string[] = [];
  const usage = (status: string, page: string, reason_code?: string) => ({ status, source: 'official_docs', source_url: `https://developers.openai.com/api/docs/models/${page}`, checked_at: '2026-11-01', reason_code });
  const entries = [
    { id: 'tts-1', vision: usage('unsupported', 'tts-1', 'AI_VISION_IMAGE_INPUT_UNSUPPORTED') },
    ...(!empty ? [
      { id: 'gpt-5.4-mini-2026-03-17', vision: usage('supported', 'gpt-5.4-mini') },
      { id: 'gpt-6-luna', vision: usage('supported', 'gpt-6-luna') },
      ...Array.from({ length: 12 }, (_, i) => ({ id: `verified-vision-${i}`, vision: { status: 'supported', source: 'provider_api' } })),
      ...Array.from({ length: 112 }, (_, i) => ({ id: `online-unknown-${i}`, vision: { status: 'unknown', source: 'unknown' } })),
      { id: 'gpt-image-2', vision: usage('unsupported', 'gpt-image-2', 'AI_VISION_TEXT_OUTPUT_UNSUPPORTED') },
      { id: 'gpt-5.4-pro-2026-03-05', vision: usage('unsupported', 'gpt-5.4-pro', 'AI_VISION_PROTOCOL_UNSUPPORTED') },
    ] : []),
    { id: 'future-native', vision: { status: 'unknown', source: 'unknown' } },
  ].map(item => ({ ...item, label: item.id, modalities: [], reported_modalities: [], manual_modalities: [], match: 'unmatched' }));
  page.on('pageerror', error => errors.push(error.message));
  await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const url = new URL(route.request().url()), path = url.pathname;
    if (path.includes('/_nuxt_icon/')) return route.fallback();
    let data: unknown = {};
    if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, install_status: 'installed' };
    else if (path.endsWith('/me/context')) data = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, display_name: '视觉型号验收', status: 1, is_root: true }, members: [{ tenant_uuid: tenant, tenant_name: '验收', member_id: 1, is_admin: true }] };
    else if (path.endsWith('/menus')) data = { menus: [{ id: 'ai', title: '模型配置', path: '/settings/ai', visible: true, origin: 'system' }] };
    else if (path.endsWith('/agents/providers')) data = { providers: [{ id: 'openai', name: 'OpenAI', configured: true, auth: { scheme: 'bearer', fields: ['api_key', 'base_url'], defaults: { base_url: 'https://provider.example.invalid/v1' } } }] };
    else if (path.endsWith('/agents/settings/profiles')) data = { env: 'dev', profiles: [image, ...(vision ? [vision] : [])] };
    else if (path.endsWith('/agents/settings/credentials')) data = { credentials: [{ provider: 'openai', data: { base_url: 'https://provider.example.invalid/v1' } }] };
    else if (path.endsWith('/agents/settings/active')) data = { profile: url.searchParams.get('modality') === 'vlm' ? vision : url.searchParams.get('modality') === 'image' ? image : null };
    else if (path.endsWith('/agents/models')) data = { models: entries.map(item => item.id), catalog_source: 'online', items: entries, updated_at: '2026-11-01T00:00:00Z' };
    else if (path.endsWith('/agents/models/sync')) data = { provider: 'openai', env: 'dev', app: '', protocol: 'openai', source: 'https://provider.example.invalid/v1/models', updated_at: '2026-11-01T00:00:00Z', revision: 'vision-choice-proof', models: entries };
    else if (path.endsWith('/agents/settings/save')) {
      const body = route.request().postDataJSON(); saved.push(body);
      vision = { ...vision, env: 'dev', modality: 'vlm', provider: 'openai', model: body.vlm.model, defaults: { max_tokens: Number(body.vlm.maxTokens) } };
      data = { ok: true };
    }
    else if (path.endsWith('/skills/source-policy')) data = { allowlist: ['builtin', 'plugin'], effective_source: 'default' };
    else if (path.includes('/tenant/invocations') || path.includes('/test/call')) inference.push(path);
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data }) });
  });
  return { saved, errors, inference, image, originalImage };
}

async function closeNavigation(page: Page, narrow: boolean) {
  if (!narrow) return;
  const backdrop = page.locator('[class~="fixed"][class~="inset-0"][class~="z-40"][class~="md:hidden"]');
  if (await backdrop.isVisible()) await backdrop.click({ position: { x: 380, y: 100 } });
}

for (const narrow of [false, true]) {
  test(`vision choices explain evidence and preserve bindings (${narrow ? 'narrow' : 'desktop'})`, async ({ page }, info) => {
    test.setTimeout(240_000);
    await page.setViewportSize(narrow ? { width: 390, height: 844 } : { width: 1440, height: 1000 });
    const f = await fixture(page);
    await page.goto('/settings/ai?modality=vlm');
    const model = page.getByTestId('ai-model-select');
    await expect(model).toContainText('tts-1', { timeout: 90_000 });
    await closeNavigation(page, narrow);
    await expect(page.getByTestId('ai-vision-model-status')).toContainText('不支持图片输入');
    await expect(page.getByTestId('ai-save-settings')).toBeDisabled();
    await expect(page.getByTestId('ai-vision-model-groups')).toHaveCount(0);
    await model.click();
    await expect(page.getByTestId('ai-vision-models-supported')).toHaveText(/推荐视觉\s*14/);
    await expect(page.getByTestId('ai-vision-models-other')).toHaveText(/其他型号\s*116/);
    await expect(page.getByRole('option', { name: /^gpt-5.4-mini-2026-03-17/ })).toBeVisible();
    await expect(page.getByRole('option', { name: /^future-native/ })).toHaveCount(0);
    await expect(page.getByRole('option', { name: /^tts-1/ })).toHaveAttribute('data-disabled', '');
    await page.getByRole('option', { name: /^gpt-5.4-mini-2026-03-17/ }).click();
    await expect(page.getByTestId('ai-vision-model-status')).toContainText('支持图片识别');
    await expect(page.getByTestId('ai-model-details')).toHaveCount(0);
    await page.getByTestId('ai-model-details-toggle').click();
    await expect(page.getByTestId('ai-model-details').getByRole('link')).toHaveAttribute('href', 'https://developers.openai.com/api/docs/models/gpt-5.4-mini');
    await page.getByTestId('ai-model-details-toggle').click();
    await model.click(); await page.getByTestId('ai-vision-models-other').click(); await expect(model).toContainText('gpt-5.4-mini');
    await expect(page.getByRole('option', { name: /^gpt-image-2/ })).toHaveAttribute('data-disabled', '');
    await expect(page.getByRole('option', { name: /^gpt-5.4-pro/ })).toContainText('尚未适配');
    await page.getByRole('option', { name: /^gpt-image-2/ }).click({ force: true });
    await expect(model).toContainText('gpt-5.4-mini');
    await page.getByRole('option', { name: /^future-native/ }).click();
    await expect(page.getByTestId('ai-vision-model-status')).toContainText('用途待确认');
    await model.click(); await page.getByTestId('ai-vision-models-supported').click(); await expect(model).toContainText('future-native'); await page.getByRole('option', { name: /^gpt-5.4-mini-2026-03-17/ }).click();
    const budget = page.getByTestId('vlm-max-tokens');
    await expect(page.getByRole('listbox')).toBeHidden(); await expect(model).toBeFocused();
    await budget.click(); await expect(budget).toBeFocused(); await budget.fill('4096'); await expect(budget).toHaveValue('4096');
    const response = page.waitForResponse(response => response.url().includes('/agents/settings/save') && response.request().method() === 'POST');
    await page.getByTestId('ai-save-settings').click(); expect((await response).status()).toBe(200);
    await expect(page.getByText('视觉分析 配置已更新', { exact: true })).toBeVisible();
    expect(f.saved[0].vlm).toMatchObject({ model: 'gpt-5.4-mini-2026-03-17', maxTokens: 4096, reuseCredential: true });
    await page.reload(); await expect(model).toContainText('gpt-5.4-mini', { timeout: 90_000 }); await closeNavigation(page, narrow);
    await expect(budget).toHaveValue('4096');
    await expect(page.getByTestId('ai-vision-model-groups')).toHaveCount(0);
    await expect(page.getByTestId('ai-model-details')).toHaveCount(0);
    if (!narrow) {
      const providerBox = await page.getByTestId('ai-provider-select').boundingBox(), modelBox = await model.boundingBox();
      expect(Math.abs(providerBox!.y - modelBox!.y)).toBeLessThanOrEqual(1);
      expect(Math.abs(providerBox!.height - modelBox!.height)).toBeLessThanOrEqual(1);
      expect((await page.getByTestId('ai-base-url').boundingBox())!.y - modelBox!.y).toBeLessThan(155);
    }
    await model.click(); await expect(page.getByRole('option', { name: /^tts-1/ })).toHaveCount(0);
    await page.screenshot({ path: info.outputPath(`vision-picker-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true, animations: 'disabled' });
    const popup = await page.getByRole('listbox').boundingBox(); expect(popup!.x).toBeGreaterThanOrEqual(0); expect(popup!.x + popup!.width).toBeLessThanOrEqual(narrow ? 390 : 1440);
    await page.keyboard.press('Escape');
    if (narrow) await page.getByTestId('ai-provider-model-form').scrollIntoViewIfNeeded();
    await page.screenshot({ path: info.outputPath(`vision-choices-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true, animations: 'disabled' });
    expect(await page.locator('body').evaluate(element => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
    await page.getByRole('button', { name: '图像生成', exact: true }).click(); await expect(model).toContainText('gpt-image-2');
    await page.getByRole('button', { name: '视觉分析', exact: true }).click(); await expect(model).toContainText('gpt-5.4-mini');
    expect(JSON.stringify(f.image)).toBe(f.originalImage); expect(f.inference).toEqual([]); expect(f.errors).toEqual([]);
  });
}

test('no confirmed vision model never defaults to first online ID', async ({ page }) => {
  test.setTimeout(240_000);
  const f = await fixture(page, true);
  await page.goto('/settings/ai?modality=vlm');
  await expect(page.getByTestId('ai-model-select')).toBeEnabled({ timeout: 90_000 });
  await page.getByTestId('ai-model-select').click();
  await expect(page.getByTestId('ai-vision-models-supported')).toHaveText(/推荐视觉\s*0/);
  await expect(page.getByTestId('ai-model-select')).not.toContainText('tts-1');
  await expect(page.getByTestId('ai-save-settings')).toBeDisabled();
  await page.getByTestId('ai-vision-models-other').click();
  await expect(page.getByRole('option', { name: /^tts-1/ })).toHaveAttribute('data-disabled', '');
  await page.getByRole('option', { name: /^future-native/ }).click();
  await expect(page.getByTestId('ai-save-settings')).toBeEnabled(); expect(f.inference).toEqual([]);
});


test('compact desktop selector keeps alignment and details out of the default form', async ({ page }, info) => {
  test.setTimeout(240_000);
  await page.setViewportSize({ width: 1280, height: 900 });
  await fixture(page);
  await page.goto('/settings/ai?modality=vlm');
  const model = page.getByTestId('ai-model-select');
  await expect(model).toContainText('tts-1', { timeout: 90_000 });
  await model.click(); await page.getByRole('option', { name: /^gpt-5.4-mini-2026-03-17/ }).click();
  await expect(page.getByRole('listbox')).toBeHidden();
  const providerBox = await page.getByTestId('ai-provider-select').boundingBox(), modelBox = await model.boundingBox();
  expect(Math.abs(providerBox!.y - modelBox!.y)).toBeLessThanOrEqual(1);
  await expect(page.getByTestId('ai-vision-model-groups')).toHaveCount(0);
  await expect(page.getByTestId('ai-model-details')).toHaveCount(0);
  expect((await page.getByTestId('ai-provider-model-form').boundingBox())!.height).toBeLessThan(340);
  await page.screenshot({ path: info.outputPath('model-selector-compact-desktop.png'), fullPage: true, animations: 'disabled' });
  await model.click();
  await expect(page.getByTestId('ai-vision-models-other')).toHaveText(/其他型号\s*116/);
  await expect.poll(async () => (await page.getByRole('listbox').boundingBox())!.width).toBeGreaterThanOrEqual(340);
  const popup = await page.getByRole('listbox').boundingBox();
  expect(popup!.x + popup!.width).toBeLessThanOrEqual(1280);
  await page.keyboard.press('Escape'); await expect(model).toBeFocused();
  await page.getByTestId('ai-model-details-toggle').click();
  await expect(page.getByTestId('ai-model-details').getByRole('link')).toBeVisible();
  await page.getByTestId('ai-model-details-toggle').click();
  await expect(model).toContainText('gpt-5.4-mini');
});
