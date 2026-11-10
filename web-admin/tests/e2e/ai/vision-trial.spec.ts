import { test, expect } from '@playwright/test';

for (const narrow of [false, true]) {
  test(`image trial upload, URL, error recovery and cancellation (${narrow ? 'narrow' : 'desktop'})`, async ({ page }, info) => {
    test.setTimeout(240_000);
    await page.setViewportSize(narrow ? { width: 390, height: 844 } : { width: 1440, height: 1000 });
    const tenant = '00000000-0000-4000-8000-000000000001';
    await page.addInitScript(tenant => {
      const expiresAt = Date.now() + 3600_000;
      const token = `header.${btoa(JSON.stringify({ exp: Math.floor(expiresAt / 1000), tid: tenant }))}.fixture`;
      localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'vision-trial-proof', revision: 1, accessToken: token, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: tenant }));
      localStorage.setItem('welcome_guide_completed', 'true'); localStorage.setItem('px_locale', 'zh');
    }, tenant);
    const vision = { env: 'dev', modality: 'vlm', provider: 'openai', model: 'gpt-6-luna', defaults: { max_tokens: 2048, reasoning_effort: 'none', json_mode: false } };
    const image = { env: 'dev', modality: 'image', provider: 'openai', model: 'gpt-image-2', defaults: { size: '1536x1024', quality: 'high', format: 'webp' } };
    const originalImage = JSON.stringify(image);
    const executions: any[] = [], unexpectedWrites: string[] = [], errors: string[] = [];
    let fail = false, hold = false, release: (() => void) | undefined;
    page.on('pageerror', error => errors.push(error.message));
    await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const url = new URL(route.request().url()), path = url.pathname;
      if (path.includes('/_nuxt_icon/')) return route.fallback();
      let data: unknown = {};
      if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, install_status: 'installed' };
      else if (path.endsWith('/me/context')) data = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, display_name: '图片试跑验收', status: 1, is_root: true }, members: [{ tenant_uuid: tenant, tenant_name: '验收', member_id: 1, is_admin: true }] };
      else if (path.endsWith('/menus')) data = { menus: [{ id: 'ai', title: '模型配置', path: '/settings/ai', visible: true, origin: 'system' }] };
      else if (path.endsWith('/agents/providers')) data = { providers: [{ id: 'openai', name: 'OpenAI', configured: true, auth: { scheme: 'bearer', fields: ['api_key', 'base_url'], defaults: { base_url: 'https://provider.example.invalid/v1' } } }] };
      else if (path.endsWith('/agents/settings/profiles')) data = { env: 'dev', profiles: [image] };
      else if (path.endsWith('/agents/settings/credentials')) data = { credentials: [{ provider: 'openai', data: { base_url: 'https://provider.example.invalid/v1' } }] };
      else if (path.endsWith('/agents/settings/active')) data = { profile: url.searchParams.get('modality') === 'vlm' ? null : url.searchParams.get('modality') === 'image' ? image : null };
      else if (path.endsWith('/agents/models')) data = { models: ['gpt-6-luna', 'unsaved-vision-model', 'gpt-image-2'], catalog_source: 'online', items: [] };
      else if (path.endsWith('/agents/test/call')) {
        const body = route.request().postDataJSON(); executions.push(body);
        if (hold) await new Promise<void>(resolve => { release = resolve; });
        if (fail) return route.fulfill({ status: 429, contentType: 'application/json', body: JSON.stringify({ code: 429, error_code: 'AI_PROVIDER_REJECTED', request_id: 'trial-error-trace', details: { upstream_status: 429, provider_trace_id: 'provider-trace' } }) });
        data = { ok: true, generation_submitted: true, result: { type: 'analysis', provider: 'openai', model: body.vlm.model, text: 'The image contains a blue square and the text TEST.', trace_id: 'trial-success-trace', elapsed_ms: 1250, env: 'dev', usage: { total_tokens: 12 } } };
      }
      else if (path.endsWith('/skills/source-policy')) data = { allowlist: ['builtin', 'plugin'], effective_source: 'default' };
      else if (route.request().method() !== 'GET') unexpectedWrites.push(path);
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data }) });
    });
    await page.goto('/settings/ai?modality=vlm');
    await expect(page.getByTestId('ai-provider-select')).toBeEnabled({ timeout: 90_000 });
    await expect(page.getByTestId('ai-model-select')).toBeEnabled();
    if (narrow) { const backdrop = page.locator('[class~="fixed"][class~="inset-0"][class~="z-40"][class~="md:hidden"]'); if (await backdrop.isVisible()) await backdrop.click({ position: { x: 380, y: 100 } }); }
    await page.getByTestId('ai-model-select').click(); await page.getByTestId('ai-vision-models-other').click();
    await page.getByRole('option', { name: 'gpt-6-luna', exact: true }).click();
    await expect(page.getByRole('listbox')).toBeHidden(); await expect(page.getByTestId('ai-model-select')).toBeFocused();
    await page.getByTestId('ai-provider-model-form').locator('input[type="password"]').fill('draft-ui-key');
    await page.getByTestId('ai-base-url').fill('https://draft-ui.example.invalid/v1'); await page.keyboard.press('Tab');
    await page.getByTestId('ai-vision-trial').click(); const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible(); expect(executions).toHaveLength(0);
    await dialog.getByTestId('vision-trial-file').setInputFiles({ name: 'blue.png', mimeType: 'image/png', buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aU1cAAAAASUVORK5CYII=', 'base64') });
    await expect(dialog.getByTestId('vision-trial-preview')).toBeVisible();
    await dialog.getByTestId('vision-trial-prompt').fill('Describe this image and read the text.');
    await dialog.getByRole('tab', { name: '识别结果', exact: true }).click(); await dialog.getByRole('tab', { name: '图片与要求', exact: true }).click();
    await expect(dialog.getByTestId('vision-trial-preview')).toBeVisible(); expect(executions).toHaveLength(0);
    await dialog.getByTestId('vision-trial-run').click(); await expect(dialog.getByTestId('vision-trial-analysis')).toContainText('blue square');
    expect(executions[0]).toMatchObject({ env: 'dev', modality: 'vlm', execute: true, vlm: { provider: 'openai', model: 'gpt-6-luna', apiKey: 'draft-ui-key', baseURL: 'https://draft-ui.example.invalid/v1', maxTokens: 2048 } });
    expect(executions[0].inputs[1]).toMatchObject({ type: 'image_base64', content: expect.stringContaining('data:image/png;base64,') });
    expect(executions[0].useFormConnection).toBe(true);
    await expect(dialog).not.toContainText('尚未保存');
    await dialog.locator('details summary').click(); await expect(dialog).toContainText('trial-success-trace');
    await page.screenshot({ path: info.outputPath(`vision-trial-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true, animations: 'disabled' });
    const bounds = await dialog.boundingBox(); expect(bounds!.x).toBeGreaterThanOrEqual(0); expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(narrow ? 390 : 1440);
    expect(await dialog.evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1);
    await dialog.getByRole('tab', { name: '图片与要求', exact: true }).click();
    await dialog.getByRole('radio', { name: '图片 URL', exact: true }).click();
    await dialog.getByTestId('vision-trial-url').fill('https://example.invalid/trial.png');
    fail = true; await dialog.getByTestId('vision-trial-run').click(); await expect(dialog.getByTestId('vision-trial-error')).toContainText('供应商拒绝');
    await dialog.locator('[data-testid="vision-trial-error"] summary').click(); await expect(dialog).toContainText('trial-error-trace');
    await expect(dialog.getByTestId('vision-trial-url')).toHaveValue('https://example.invalid/trial.png');
    fail = false; hold = true; await dialog.getByTestId('vision-trial-run').click(); await expect.poll(() => executions.length).toBe(3);
    await expect(dialog.getByTestId('vision-trial-run')).toBeDisabled(); await dialog.getByRole('button', { name: '完成', exact: true }).click(); await expect(dialog).toHaveCount(0);
    release?.(); await page.getByTestId('ai-vision-trial').click(); await expect(dialog.getByTestId('vision-trial-analysis')).toHaveCount(0);
    await expect(dialog.getByTestId('vision-trial-url')).toHaveValue('https://example.invalid/trial.png');
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    hold = false;
    await page.getByTestId('ai-model-select').click(); await page.getByRole('option', { name: 'unsaved-vision-model', exact: true }).click();
    await expect(page.getByRole('listbox')).toBeHidden(); await expect(page.getByTestId('ai-model-select')).toBeFocused();
    await page.getByTestId('ai-provider-model-form').locator('input[type="password"]').fill('');
    await page.getByTestId('ai-base-url').fill(''); await page.keyboard.press('Tab');
    await page.getByTestId('ai-vision-trial').click();
    await expect(dialog.getByTestId('vision-trial-url')).toHaveValue('https://example.invalid/trial.png');
    await dialog.getByTestId('vision-trial-run').click(); await expect(dialog.getByTestId('vision-trial-analysis')).toContainText('blue square');
    expect(executions[3]).toMatchObject({ useFormConnection: true, vlm: { model: 'unsaved-vision-model', baseURL: '' } });
    expect(executions[3].vlm).not.toHaveProperty('apiKey');
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await page.getByRole('button', { name: '图像生成', exact: true }).click(); await expect(page.getByTestId('ai-model-select')).toContainText('gpt-image-2');
    await expect(page.getByTestId('ai-vision-trial')).toHaveCount(0);
    expect(JSON.stringify(image)).toBe(originalImage); expect(unexpectedWrites).toEqual([]); expect(errors).toEqual([]);
  });
}
