import { test, expect } from '@playwright/test';

for (const narrow of [false, true]) {
  test(`saved OpenAI credentials are reused across modalities (${narrow ? 'narrow' : 'desktop'})`, async ({ page }, info) => {
    test.setTimeout(120_000);
    await page.setViewportSize(narrow ? { width: 390, height: 844 } : { width: 1440, height: 1000 });
    const tenant = '00000000-0000-4000-8000-000000000001';
    await page.addInitScript(tenant => {
      const expiresAt = Date.now() + 3600_000;
      const token = `header.${btoa(JSON.stringify({ exp: Math.floor(expiresAt / 1000), tid: tenant }))}.fixture`;
      localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'shared-provider-proof', revision: 1, accessToken: token, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: tenant }));
      localStorage.setItem('welcome_guide_completed', 'true'); localStorage.setItem('px_locale', 'zh');
    }, tenant);
    const profiles: any[] = [{ modality: 'llm', provider: 'openai', model: 'gpt-4o', defaults: { maxTokens: 64 } }];
    const saves: any[] = [], connections: any[] = [], errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const url = new URL(route.request().url()), path = url.pathname;
      if (path.includes('/_nuxt_icon/')) return route.fallback();
      let data: any = {};
      if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, install_status: 'installed' };
      else if (path.endsWith('/me/context')) data = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, display_name: '共享凭据验收', status: 1, is_root: true }, members: [{ tenant_uuid: tenant, tenant_name: '验收', member_id: 1, is_admin: true }] };
      else if (path.endsWith('/menus')) data = { menus: [{ id: 'ai', title: '模型配置', path: '/settings/ai', visible: true, origin: 'system' }] };
      else if (path.endsWith('/agents/providers')) data = { providers: ['openai', 'agnes'].map(id => ({ id, name: id === 'openai' ? 'OpenAI' : 'Agnes AI', auth: { scheme: 'bearer', fields: ['api_key', 'base_url'], defaults: { base_url: 'https://provider.example.invalid/v1' } } })) };
      else if (path.endsWith('/agents/settings/credentials')) data = { env: 'dev', credentials: [{ provider: 'openai', has_credentials: true, data: { base_url: '' } }] };
      else if (path.endsWith('/agents/settings/profiles')) data = { env: 'dev', profiles };
      else if (path.endsWith('/agents/settings/active')) data = { profile: profiles.find(profile => profile.modality === url.searchParams.get('modality')) || null };
      else if (path.endsWith('/agents/models')) data = { models: url.searchParams.get('modality') === 'image' ? ['gpt-image-1'] : ['gpt-4o'], catalog_source: 'online', items: [] };
      else if (path.endsWith('/test/connection')) { connections.push(route.request().postDataJSON()); data = { ok: true, verification: { api_key_verified: true, generation_submitted: false } }; }
      else if (path.endsWith('/agents/settings/save')) {
        const body = route.request().postDataJSON(); saves.push(body);
        profiles.push({ modality: body.modality, provider: body[body.modality].provider, model: body[body.modality].model, defaults: {} });
        data = { ok: true };
      }
      else if (path.endsWith('/skills/source-policy')) data = { allowlist: ['builtin', 'plugin'], effective_source: 'default' };
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data }) });
    });
    const closeNavigation = async () => {
      const backdrop = page.locator('[class~="fixed"][class~="inset-0"][class~="z-40"][class~="md:hidden"]');
      if (await backdrop.isVisible()) await backdrop.click({ position: { x: 380, y: 100 } });
    };
    await page.goto('/settings/ai');
    const model = page.getByTestId('ai-model-select'), key = page.getByTestId('ai-api-key');
    await expect(model).toContainText('gpt-4o', { timeout: 90_000 });
    await expect(key).toHaveValue('');
    await expect(page.getByTestId('ai-shared-credential-hint')).toContainText('已保存 OpenAI 公共凭据');
    await closeNavigation();
    await page.getByRole('button', { name: '图像生成', exact: true }).click();
    await expect(model).toContainText('gpt-image-1'); await expect(model).toBeEnabled();
    await expect(key).toHaveAttribute('placeholder', '留空使用已保存的凭据');
    expect(await key.getAttribute('required')).toBeNull();
    await expect(key).toHaveValue('');
    await page.getByRole('button', { name: '测试连接', exact: true }).click();
    await expect.poll(() => connections.length).toBe(1);
    expect(connections[0].image.apiKey).toBe('');
    await page.getByTestId('ai-save-settings').click(); await expect.poll(() => saves.length).toBe(1);
    expect(saves[0]).toMatchObject({ modality: 'image', image: { provider: 'openai', apiKey: '', model: 'gpt-image-1' } });
    await expect(page.getByText('图像生成 配置已更新', { exact: true })).toBeVisible();
    await page.goto('/settings/ai?modality=image'); await expect(model).toContainText('gpt-image-1'); await expect(model).toBeEnabled(); await closeNavigation();
    await expect(key).toHaveValue('');
    await expect(page.getByTestId('ai-shared-credential-hint')).toContainText('已保存 OpenAI 公共凭据');
    await key.click(); await expect(key).toBeFocused(); await key.fill('new-key-fixture'); await expect(key).toHaveValue('new-key-fixture');
    await expect(page.getByTestId('ai-shared-credential-hint')).toContainText('将更新当前租户、环境的 OpenAI 公共凭据');
    await page.getByRole('button', { name: 'LLM 文本', exact: true }).click(); await expect(model).toContainText('gpt-4o'); await expect(model).toBeEnabled();
    await expect(key).toHaveValue('');
    await page.getByRole('button', { name: '图像生成', exact: true }).click(); await expect(model).toContainText('gpt-image-1'); await expect(model).toBeEnabled();
    await expect(key).toHaveValue('new-key-fixture'); await key.fill(''); await expect(page.getByTestId('ai-shared-credential-hint')).toContainText('已保存 OpenAI 公共凭据');
    await page.getByTestId('ai-provider-select').click(); await page.getByRole('option', { name: 'Agnes AI', exact: true }).click();
    await expect(page.getByTestId('ai-shared-credential-hint')).toHaveCount(0);
    await expect(key).toHaveValue(''); await expect(key).toHaveAttribute('required', '');
    await page.getByTestId('ai-provider-select').click(); await page.getByRole('option', { name: 'OpenAI', exact: true }).click();
    await expect(page.getByTestId('ai-shared-credential-hint')).toContainText('已保存 OpenAI 公共凭据');
    await page.getByTestId('ai-shared-credential-hint').evaluate(element => element.scrollIntoView({ block: 'center' }));
    const form = page.getByTestId('ai-provider-model-form');
    expect(await form.evaluate(element => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
    await page.screenshot({ path: info.outputPath(`shared-credentials-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true, animations: 'disabled' });
    expect(errors).toEqual([]);
  });
}
