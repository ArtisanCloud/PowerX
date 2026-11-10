import { test, expect } from '@playwright/test';

for (const narrow of [false, true]) {
  test(`online model IDs can be selected and saved without local registration (${narrow ? 'narrow' : 'desktop'})`, async ({ page }, info) => {
    test.setTimeout(240_000);
    await page.setViewportSize(narrow ? { width: 390, height: 844 } : { width: 1440, height: 1000 });
    const tenant = '00000000-0000-4000-8000-000000000001';
    await page.addInitScript(tenant => {
      const expiresAt = Date.now() + 3600_000;
      const token = `header.${btoa(JSON.stringify({ exp: Math.floor(expiresAt / 1000), tid: tenant }))}.fixture`;
      localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'online-first-proof', revision: 1, accessToken: token, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: tenant }));
      localStorage.setItem('welcome_guide_completed', 'true'); localStorage.setItem('px_locale', 'zh');
    }, tenant);
    const image = { env: 'dev', modality: 'image', provider: 'openai', model: 'gpt-image-2', defaults: { size: '1536x1024', quality: 'high', format: 'webp' } };
    const originalImage = JSON.stringify(image);
    let vision = { env: 'dev', modality: 'vlm', provider: 'openai', model: 'gpt-6-luna', defaults: { max_tokens: 2048, json_mode: false } };
    let refreshed = false;
    const saved: any[] = [], syncs: any[] = [], generation: string[] = [], errors: string[] = [];
    const models = () => ['gpt-6-luna', 'fresh-vision-native', ...(refreshed ? ['fresh-newer-native'] : [])];
    const entries = () => models().map(id => ({ id, label: id, modalities: [], reported_modalities: [], manual_modalities: [], match: 'unmatched' }));
    page.on('pageerror', error => errors.push(error.message));
    await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const url = new URL(route.request().url()), path = url.pathname;
      if (path.includes('/_nuxt_icon/')) return route.fallback();
      let data: unknown = {};
      if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, install_status: 'installed' };
      else if (path.endsWith('/me/context')) data = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, display_name: '在线目录验收', status: 1, is_root: true }, members: [{ tenant_uuid: tenant, tenant_name: '验收', member_id: 1, is_admin: true }] };
      else if (path.endsWith('/menus')) data = { menus: [{ id: 'ai', title: '模型配置', path: '/settings/ai', visible: true, origin: 'system' }] };
      else if (path.endsWith('/agents/providers')) data = { providers: [{ id: 'openai', name: 'OpenAI', configured: true, auth: { scheme: 'bearer', fields: ['api_key', 'base_url'], defaults: { base_url: 'https://provider.example.invalid/v1' } } }] };
      else if (path.endsWith('/agents/settings/profiles')) data = { env: 'dev', profiles: [vision, image] };
      else if (path.endsWith('/agents/settings/credentials')) data = { credentials: [{ provider: 'openai', data: { base_url: 'https://provider.example.invalid/v1' } }] };
      else if (path.endsWith('/agents/settings/active')) data = { profile: url.searchParams.get('modality') === 'vlm' ? vision : url.searchParams.get('modality') === 'image' ? image : null };
      else if (path.endsWith('/agents/models/sync')) {
        syncs.push(route.request().postDataJSON()); refreshed = true;
        data = { provider: 'openai', env: 'dev', app: '', protocol: 'openai', source: 'https://provider.example.invalid/v1/models', updated_at: '2026-11-01T00:00:00Z', revision: 'online-proof', models: entries() };
      }
      else if (path.endsWith('/agents/models')) data = { models: models(), catalog_source: 'online', updated_at: '2026-11-01T00:00:00Z', items: entries() };
      else if (path.endsWith('/agents/settings/save')) {
        const body = route.request().postDataJSON(); saved.push(body);
        vision = { ...vision, model: body.vlm.model, defaults: { max_tokens: Number(body.vlm.maxTokens), json_mode: body.vlm.jsonMode } };
        data = { ok: true };
      }
      else if (path.endsWith('/skills/source-policy')) data = { allowlist: ['builtin', 'plugin'], effective_source: 'default' };
      else if (path.includes('/tenant/invocations') || path.includes('/test/call')) generation.push(path);
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data }) });
    });
    const closeMobileNavigation = async () => {
      if (!narrow) return;
      const backdrop = page.locator('[class~="fixed"][class~="inset-0"][class~="z-40"][class~="md:hidden"]');
      if (await backdrop.isVisible()) await backdrop.click({ position: { x: 380, y: 100 } });
    };
    await page.goto('/settings/ai?modality=vlm');
    const model = page.getByTestId('ai-model-select');
    await expect(model).toContainText('gpt-6-luna', { timeout: 90_000 }); await expect(model).toBeEnabled();
    await closeMobileNavigation();
    await expect(page.getByTestId('ai-model-source')).toContainText('在线目录');
    await model.click(); await expect(page.getByRole('option', { name: /^fresh-vision-native/ })).toBeVisible();
    await expect(page.getByRole('option', { name: 'gpt-4o', exact: true })).toHaveCount(0);
    await page.getByRole('option', { name: /^fresh-vision-native/ }).click();
    await expect(page.getByTestId('ai-vision-model-status')).toContainText('用途待确认');
    await expect(page.getByRole('listbox')).toBeHidden();
    const budget = page.getByTestId('vlm-max-tokens'); await expect(budget).toBeEnabled();
    await budget.click(); await expect(budget).toBeFocused(); await budget.fill('4096');
    await expect(budget).toHaveValue('4096');
    const reply = page.waitForResponse(response => response.url().includes('/agents/settings/save') && response.request().method() === 'POST');
    await page.getByTestId('ai-save-settings').click(); expect((await reply).status()).toBe(200);
    await expect(page.getByText('视觉分析 配置已更新', { exact: true })).toBeVisible();
    expect(saved[0].vlm).toMatchObject({ model: 'fresh-vision-native', maxTokens: 4096, reuseCredential: true });
    expect(saved[0].vlm).not.toHaveProperty('apiKey'); expect(saved[0].vlm).not.toHaveProperty('reasoningEffort');
    await page.reload(); await expect(model).toContainText('fresh-vision-native', { timeout: 90_000 });
    await closeMobileNavigation(); await expect(budget).toHaveValue('4096');
    await page.getByTestId('ai-online-models').click(); const dialog = page.getByRole('dialog');
    await expect(dialog.getByTestId('online-model-summary')).toContainText('在线获取 3 个型号');
    await expect(dialog.getByTestId('online-model-list')).toContainText('fresh-newer-native');
    await expect(dialog.getByTestId('online-model-list')).toContainText('用途未声明');
    await expect(dialog.getByTestId('online-model-assign')).toHaveCount(0);
    await page.screenshot({ path: info.outputPath(`online-first-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true, animations: 'disabled' });
    const bounds = await dialog.boundingBox(); expect(bounds!.x).toBeGreaterThanOrEqual(0); expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(narrow ? 390 : 1440);
    expect(await dialog.evaluate(element => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
    await dialog.getByRole('button', { name: '完成', exact: true }).click(); await expect(model).toContainText('fresh-vision-native');
    await model.click(); await expect(page.getByRole('option', { name: /^fresh-newer-native/ })).toBeVisible(); await page.keyboard.press('Escape');
    await page.getByRole('button', { name: '图像生成', exact: true }).click(); await expect(model).toContainText('gpt-image-2');
    expect(JSON.stringify(image)).toBe(originalImage); expect(generation).toEqual([]); expect(errors).toEqual([]);
    expect(syncs).toEqual([{ env: 'dev', provider: 'openai', app: '', modality: 'vlm', action: 'refresh' }]);
  });
}
