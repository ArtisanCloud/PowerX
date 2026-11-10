import { test, expect } from '@playwright/test';

for (const narrow of [false, true]) {
  test(`vision settings remain separate from generation (${narrow ? 'narrow' : 'desktop'})`, async ({ page }, info) => {
    test.setTimeout(240_000);
    await page.setViewportSize(narrow ? { width: 390, height: 844 } : { width: 1440, height: 1000 });
    const tenant = '00000000-0000-4000-8000-000000000001';
    await page.addInitScript(tenant => {
      const expiresAt = Date.now() + 3600_000;
      const token = `header.${btoa(JSON.stringify({ exp: Math.floor(expiresAt / 1000), tid: tenant }))}.fixture`;
      localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'vision-settings-proof', revision: 1, accessToken: token, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: tenant }));
      localStorage.setItem('welcome_guide_completed', 'true'); localStorage.setItem('px_locale', 'zh');
    }, tenant);
    const saved: any[] = [], checked: any[] = [], connections: any[] = [], syncs: any[] = [], generated: string[] = [], errors: string[] = [];
    const imageProfile = { env: 'dev', modality: 'image', provider: 'openai', model: 'gpt-image-2', defaults: { size: '1536x1024', quality: 'high', format: 'webp' } };
    const originalImage = JSON.stringify(imageProfile);
    let visionProfile: any = null;
    page.on('pageerror', error => errors.push(error.message));
    await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const url = new URL(route.request().url()), path = url.pathname;
      if (path.includes('/_nuxt_icon/')) return route.fallback();
      let data: unknown = {};
      if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, install_status: 'installed' };
      else if (path.endsWith('/me/context')) data = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, display_name: '视觉配置验收', status: 1, is_root: true }, members: [{ tenant_uuid: tenant, tenant_name: '验收', member_id: 1, is_admin: true }] };
      else if (path.endsWith('/menus')) data = { menus: [{ id: 'ai', title: '模型配置', path: '/settings/ai', visible: true, origin: 'system' }] };
      else if (path.endsWith('/agents/providers')) data = { providers: [{ id: 'openai', name: 'OpenAI', configured: true, auth: { scheme: 'bearer', fields: ['api_key', 'base_url'], defaults: { base_url: 'https://openai.example.invalid/v1' } } }] };
      else if (path.endsWith('/agents/settings/profiles')) data = { env: 'dev', profiles: [imageProfile, ...(visionProfile ? [visionProfile] : [])] };
      else if (path.endsWith('/agents/settings/credentials')) data = { env: 'dev', credentials: [{ provider: 'openai', data: { base_url: 'https://openai.example.invalid/v1' } }] };
      else if (path.endsWith('/agents/settings/active')) data = { profile: url.searchParams.get('modality') === 'image' ? imageProfile : url.searchParams.get('modality') === 'vlm' ? visionProfile : null };
      else if (path.endsWith('/agents/models/sync')) {
        syncs.push(route.request().postDataJSON());
        data = { provider: 'openai', env: 'dev', app: '', protocol: 'openai', source: 'https://openai.example.invalid/v1/models', updated_at: '2026-10-30T00:00:00Z', revision: 'vision-proof', models: [{ id: 'gpt-6-luna', label: 'GPT-6 Luna', modalities: ['llm', 'vlm'], manual_modalities: [], match: 'local' }, { id: 'gpt-image-2', label: 'GPT Image 2', modalities: ['image'], manual_modalities: [], match: 'local' }] };
      }
      else if (path.endsWith('/agents/models')) data = { models: url.searchParams.get('modality') === 'vlm' ? ['gpt-6-luna'] : url.searchParams.get('modality') === 'image' ? ['gpt-image-2'] : ['gpt-6-luna'] };
      else if (path.endsWith('/agents/settings/save')) {
        const body = route.request().postDataJSON(); saved.push(body);
        expect(body.modality).toBe('vlm'); expect(body.vlm.reuseCredential).toBe(true);
        expect(body.vlm).not.toHaveProperty('apiKey'); expect(body.vlm).not.toHaveProperty('baseURL');
        expect(body.vlm).not.toHaveProperty('temperature'); expect(body.vlm).not.toHaveProperty('topP');
        visionProfile = { env: 'dev', modality: 'vlm', provider: body.vlm.provider, model: body.vlm.model, defaults: { max_tokens: Number(body.vlm.maxTokens), reasoning_effort: body.vlm.reasoningEffort, json_mode: body.vlm.jsonMode } };
        data = { ok: true };
      }
      else if (path.endsWith('/agents/test/call')) { checked.push(route.request().postDataJSON()); data = { ok: true, generation_submitted: false }; }
      else if (path.endsWith('/agents/test/connection')) { connections.push(route.request().postDataJSON()); data = { ok: true, verification: { provider: 'openai', endpoint_reachable: true, api_key_verified: true, authentication: 'api_key', generation_submitted: false } }; }
      else if (path.endsWith('/skills/source-policy')) data = { allowlist: ['builtin', 'plugin'], effective_source: 'default' };
      else if (path.includes('/ai/') || path.includes('/tenant/invocations')) generated.push(path);
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data }) });
    });
    await page.goto('/settings/ai?modality=vlm');
    const closeMobileNavigation = async () => { if (narrow) {
      const backdrop = page.locator('[class~="fixed"][class~="inset-0"][class~="z-40"][class~="md:hidden"]');
      if (await backdrop.isVisible()) await backdrop.click({ position: { x: 380, y: 100 } });
    } };
    // 独立 Nuxt 开发构建首次载入要编译模块；等待页面就绪后再验收操作。
    const model = page.getByTestId('ai-model-select'); await expect(model).toContainText('gpt-6-luna', { timeout: 90_000 });
    await closeMobileNavigation();
    const budget = page.getByTestId('vlm-max-tokens'); await expect(budget).toBeEnabled();
    await page.keyboard.press('Escape'); await budget.click(); await expect(budget).toBeFocused(); await budget.fill('4096'); await expect(budget).toHaveValue('4096');
    await page.getByRole('button', { name: '图像生成', exact: true }).click();
    await expect(model).toContainText('gpt-image-2'); await expect(page.getByTestId('vlm-parameters')).toHaveCount(0);
    await page.getByRole('button', { name: '视觉分析', exact: true }).click();
    await expect(model).toContainText('gpt-6-luna'); await expect(budget).toHaveValue('4096');
    await page.getByTestId('vlm-reasoning').click(); await page.getByRole('option', { name: 'low', exact: true }).click();
    const panel = page.getByTestId('ai-test-panel');
    await panel.getByRole('button', { name: '检查参数', exact: true }).click(); await expect.poll(() => checked.length).toBe(1);
    expect(checked[0].modality).toBe('vlm'); expect(checked[0].vlm.maxTokens).toBe(4096); expect(checked[0]).not.toHaveProperty('image');
    await panel.getByRole('button', { name: '测试连接', exact: true }).click(); await expect.poll(() => connections.length).toBe(1);
    expect(connections[0].modality).toBe('vlm'); expect(connections[0].vlm.baseURL).toBe('https://openai.example.invalid/v1');
    const saveResponse = page.waitForResponse(response => response.url().includes('/agents/settings/save') && response.request().method() === 'POST');
    await page.getByTestId('ai-save-settings').click();
    expect((await saveResponse).status()).toBe(200);
    await expect(page.getByText('视觉分析 配置已更新', { exact: true })).toBeVisible();
    await expect.poll(() => saved.length).toBe(1);
    await page.reload(); await expect(model).toContainText('gpt-6-luna', { timeout: 90_000 }); await expect(budget).toHaveValue('4096');
    await closeMobileNavigation();
    await expect(page.getByTestId('vlm-reasoning')).toContainText('low');
    await page.getByTestId('ai-online-models').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByTestId('online-model-summary')).toContainText('在线获取 2 个型号');
    await expect(dialog.getByTestId('online-model-list')).toContainText('gpt-6-luna');
    await expect(dialog.getByTestId('online-model-list')).toContainText('gpt-image-2');
    await dialog.getByRole('button', { name: '更新在线模型', exact: true }).click();
    await expect.poll(() => syncs.length).toBe(2);
    expect(syncs.every(item => item.modality === 'vlm' && item.action === 'refresh')).toBe(true);
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await expect(model).toContainText('gpt-6-luna');
    await page.screenshot({ path: info.outputPath(`vision-settings-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true, animations: 'disabled' });
    expect(await page.locator('body').evaluate(element => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
    await page.getByRole('button', { name: '图像生成', exact: true }).click(); await expect(model).toContainText('gpt-image-2');
    expect(JSON.stringify(imageProfile)).toBe(originalImage); expect(generated).toEqual([]); expect(errors).toEqual([]);
  });
}
