import { test, expect } from '@playwright/test';

for (const narrow of [false, true]) {
  test(`online model update and unfiltered selection (${narrow ? 'narrow' : 'desktop'})`, async ({ page }, info) => {
    await page.setViewportSize(narrow ? { width: 390, height: 844 } : { width: 1440, height: 1000 });
    const tenant = '00000000-0000-4000-8000-000000000001';
    await page.addInitScript(tenant => {
      const expiresAt = Date.now() + 3600_000;
      const token = `header.${btoa(JSON.stringify({ exp: Math.floor(expiresAt / 1000), tid: tenant }))}.fixture`;
      localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'online-model-browser', revision: 1, accessToken: token, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: tenant }));
      localStorage.setItem('welcome_guide_completed', 'true'); localStorage.setItem('px_locale', 'zh');
    }, tenant);
    const syncRequests: any[] = [], generations: string[] = [], errors: string[] = [];
    const connectionRequests: any[] = [];
    const saveRequests: any[] = [];
    let openaiBase = 'https://old-proxy.example.invalid/v1';
    let saveOK = true, holdSave = false, releaseSave: (() => void) | undefined;
    let holdConnection = false, releaseConnection: (() => void) | undefined;
    let synced = false, authFailure = false;
    const snapshot = (provider: string) => ({ provider, env: 'dev', app: '', protocol: 'openai', source: `https://${provider}.example.invalid/v1/models`, updated_at: '2026-10-30T00:00:00Z', revision: 'revision-1', models: [
      { id: provider === 'openai' ? 'gpt-4o' : 'deepseek-chat', label: 'Existing local model', modalities: ['llm'], manual_modalities: [], match: 'local' },
      { id: 'brand-new-id', label: 'Brand new online model', modalities: [], manual_modalities: [], match: 'unmatched' },
    ] });
    page.on('pageerror', error => errors.push(error.message));
    await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const url = new URL(route.request().url()), path = url.pathname;
      if (path.includes('/_nuxt_icon/')) return route.fallback();
      let data: unknown = {};
      const profile = { env: 'dev', modality: 'llm', provider: 'openai', model: 'gpt-4o', defaults: { temperature: 0.7, maxTokens: 1024 } };
      if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, install_status: 'installed' };
      else if (path.endsWith('/me/context')) data = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, display_name: '在线目录验收', status: 1, is_root: true }, members: [{ tenant_uuid: tenant, tenant_name: '验收', member_id: 1, is_admin: true }] };
      else if (path.endsWith('/menus')) data = { menus: [{ id: 'ai', title: '模型配置', path: '/settings/ai', visible: true, origin: 'system' }] };
      else if (path.endsWith('/agents/providers')) data = { providers: ['openai', 'deepseek'].map(id => ({ id, name: id === 'openai' ? 'OpenAI' : 'DeepSeek', configured: true, auth: { scheme: 'bearer', fields: ['api_key', 'base_url'], defaults: { base_url: `https://${id}.example.invalid/v1` } } })) };
      else if (path.endsWith('/agents/settings/profiles')) data = { env: 'dev', profiles: [profile] };
      else if (path.endsWith('/agents/settings/credentials')) data = { env: 'dev', credentials: ['openai', 'deepseek'].map(provider => ({ provider, data: { base_url: provider === 'openai' ? openaiBase : `https://${provider}.example.invalid/v1` } })) };
      else if (path.endsWith('/agents/settings/active')) data = { profile };
      else if (path.endsWith('/agents/models/sync')) {
        const body = route.request().postDataJSON(); syncRequests.push(body);
        if (authFailure) return route.fulfill({ status: 422, contentType: 'application/json', body: JSON.stringify({ code: 422, error_code: 'AI_MODEL_SYNC_AUTH_FAILED', message: 'AI_MODEL_SYNC_AUTH_FAILED', details: { trace_id: 'fixture-catalog-trace', source: 'https://provider.example.invalid/v1/models', upstream_status: 403, upstream_reason: 'legacy_free_key_retired' } }) });
        synced = true;
        data = snapshot(body.provider);
      }
      else if (path.endsWith('/agents/models')) data = { models: [url.searchParams.get('provider') === 'deepseek' ? 'deepseek-chat' : 'gpt-4o', ...(synced && url.searchParams.get('provider') === 'openai' ? ['brand-new-id'] : [])] };
      else if (path.endsWith('/agents/test/connection')) {
        const body = route.request().postDataJSON(); connectionRequests.push(body);
        if (holdConnection) await new Promise<void>(resolve => { releaseConnection = resolve; });
        data = { ok: true, verification: { provider: body[body.modality].provider, endpoint_reachable: true, api_key_verified: true, authentication: 'api_key', generation_submitted: false } };
      }
      else if (path.endsWith('/agents/settings/save')) {
        const body = route.request().postDataJSON(); saveRequests.push(body);
        if (holdSave) await new Promise<void>(resolve => { releaseSave = resolve; });
        if (saveOK) openaiBase = body.llm.baseURL;
        data = { ok: saveOK };
      }
      else if (path.includes('/agents/test/') || path.includes('/ai/llm/invoke') || path.includes('/settings/save')) generations.push(path);
      else if (path.endsWith('/skills/source-policy')) data = { allowlist: ['builtin', 'plugin'], effective_source: 'default' };
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data }) });
    });
    const model = page.getByTestId('ai-model-select');
    const settleForm = async () => {
      await expect(model).toContainText('gpt-4o');
      await expect(page.getByRole('dialog')).toHaveCount(0);
      if (narrow) {
        const backdrop = page.locator('[class~="fixed"][class~="inset-0"][class~="z-40"][class~="md:hidden"]');
        if (await backdrop.isVisible()) await backdrop.click({ position: { x: 380, y: 100 } });
        await expect(backdrop).toHaveCount(0);
      }
    };
    await page.goto('/settings/ai');
    await settleForm();
    await page.getByTestId('ai-online-models').click();
    const dialog = page.getByRole('dialog'); await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId('online-model-summary')).toContainText('在线获取 2 个型号；供应商已声明用途 0 个，未声明 2 个');
    expect(syncRequests).toHaveLength(1); expect(syncRequests[0]).toEqual({ provider: 'openai', env: 'dev', app: '', modality: 'llm', action: 'refresh' });
    await dialog.getByRole('tab', { name: '用途未声明 (2)', exact: true }).click();
    await expect(dialog.getByTestId('online-model-list')).toContainText('brand-new-id');
    await expect(dialog.getByTestId('online-model-assign')).toHaveCount(0);
    await expect(dialog.getByRole('checkbox')).toHaveCount(0);
    expect(syncRequests).toHaveLength(1);
    await dialog.getByRole('tab', { name: '全部在线型号 (2)', exact: true }).click();
    await page.screenshot({ path: info.outputPath(`online-models-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true, animations: 'disabled' });
    const bounds = await dialog.boundingBox(); expect(bounds!.x).toBeGreaterThanOrEqual(0); expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(narrow ? 390 : 1440);
    expect(await dialog.evaluate(element => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await expect(dialog).toHaveCount(0); await expect(model).toContainText('gpt-4o');
    await model.click(); await expect(page.getByRole('option', { name: 'brand-new-id', exact: true })).toBeVisible();
    await page.getByRole('option', { name: 'brand-new-id', exact: true }).click();
    await expect(model).toContainText('brand-new-id');
    await page.getByTestId('ai-provider-select').click(); await page.getByRole('option', { name: 'DeepSeek', exact: true }).click();
    await expect(model).toContainText('deepseek-chat'); authFailure = true;
    await page.getByTestId('ai-online-models').click();
    await expect(dialog.getByRole('alert')).toContainText('供应商已停用旧版免费 API Key');
    await expect(dialog.getByRole('alert')).toContainText('fixture-catalog-trace');
    await expect(dialog.getByRole('alert')).toContainText('https://provider.example.invalid/v1/models');
    await expect(dialog.getByRole('alert')).toContainText('HTTP 403');
    expect(await dialog.evaluate(element => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
    await page.screenshot({ path: info.outputPath(`online-model-auth-error-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true, animations: 'disabled' });
    await expect(dialog.getByTestId('online-model-summary')).toHaveCount(0);
    expect(syncRequests.at(-1).provider).toBe('deepseek');
    authFailure = false; await dialog.getByRole('button', { name: '重试', exact: true }).click();
    await expect(dialog.getByTestId('online-model-summary')).toBeVisible();
    const count = syncRequests.length; await page.waitForTimeout(1000); expect(syncRequests).toHaveLength(count);
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    const panel = page.getByTestId('ai-test-panel');
    await panel.getByRole('button', { name: '测试连接', exact: true }).click();
    await expect(panel.getByRole('status')).toContainText('API Key');
    await expect(panel.getByRole('status')).not.toContainText('暂无测试结果');
    await page.getByTestId('ai-provider-select').click(); await page.getByRole('option', { name: 'OpenAI', exact: true }).click();
    await expect(model).toContainText('brand-new-id');
    await expect(panel.getByRole('status')).toContainText('暂无测试结果');
    const baseURL = page.getByTestId('ai-base-url');
    await expect(baseURL).toHaveValue('https://old-proxy.example.invalid/v1');
    await expect(page.getByRole('listbox')).toHaveCount(0);
    await baseURL.fill('');
    await expect(baseURL).toHaveValue('');
    await expect(page.getByTestId('ai-default-base-url-hint')).toContainText('https://openai.example.invalid/v1');
    holdConnection = true;
    await panel.getByRole('button', { name: '测试连接', exact: true }).click();
    await expect.poll(() => Boolean(releaseConnection)).toBe(true);
    // Returning to the same Provider must not revive an older in-flight result.
    await page.getByTestId('ai-provider-select').click(); await page.getByRole('option', { name: 'DeepSeek', exact: true }).click();
    await expect(model).toContainText('deepseek-chat');
    await page.getByTestId('ai-provider-select').click(); await page.getByRole('option', { name: 'OpenAI', exact: true }).click();
    releaseConnection!();
    await expect(panel.getByRole('button', { name: '测试连接', exact: true })).toBeEnabled();
    await expect(panel.getByRole('status')).toContainText('暂无测试结果');
    expect(connectionRequests).toHaveLength(2);
    expect(connectionRequests[1].llm.baseURL).toBe('');
    await expect(baseURL).toHaveValue('');
    await baseURL.blur();
    await expect(model).toBeEnabled();
    await expect(page.getByRole('listbox')).toHaveCount(0);
    const apiKey = page.locator('input[type="password"]');
    await apiKey.click();
    await expect(apiKey).toBeFocused();
    await apiKey.fill('fixture-new-catalog-key');
    await expect(apiKey).toHaveValue('fixture-new-catalog-key');
    const beforeSaveSyncCount = syncRequests.length;
    await page.getByTestId('ai-online-models').click();
    await expect(dialog.getByRole('alert')).toContainText('连接配置有未保存的修改');
    expect(syncRequests).toHaveLength(beforeSaveSyncCount);
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await expect(apiKey).toHaveValue('fixture-new-catalog-key');
    await page.getByRole('button', { name: '保存', exact: true }).click();
    await expect.poll(() => saveRequests.length).toBe(1);
    expect(saveRequests[0].llm.baseURL).toBe('');
    expect(saveRequests[0].llm.apiKey).toBe('fixture-new-catalog-key');
    await expect(apiKey).toHaveValue('');
    // 保存后立即打开，无需刷新页面；目录请求仍只使用服务端保存的连接。
    await page.getByTestId('ai-online-models').click();
    await expect(dialog.getByTestId('online-model-summary')).toBeVisible();
    await expect(dialog.getByRole('alert')).toHaveCount(0);
    expect(syncRequests).toHaveLength(beforeSaveSyncCount + 1);
    expect(syncRequests.at(-1)).toEqual({ provider: 'openai', env: 'dev', app: '', modality: 'llm', action: 'refresh' });
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await page.getByTestId('ai-provider-select').click(); await page.getByRole('option', { name: 'DeepSeek', exact: true }).click();
    await expect(model).toContainText('deepseek-chat');
    await page.getByTestId('ai-provider-select').click(); await page.getByRole('option', { name: 'OpenAI', exact: true }).click();
    await expect(model).toContainText('brand-new-id');
    await expect(apiKey).toHaveValue('');
    await page.getByTestId('ai-online-models').click();
    await expect(dialog.getByTestId('online-model-summary')).toBeVisible();
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    saveOK = false;
    await apiKey.click(); await apiKey.fill('fixture-rejected-key');
    await page.getByTestId('ai-save-settings').click();
    await expect(page.getByText('保存接口未确认成功，请重试。', { exact: true }).first()).toBeVisible();
    await expect(apiKey).toHaveValue('fixture-rejected-key');
    const failedSaveSyncCount = syncRequests.length;
    await page.getByTestId('ai-online-models').click();
    await expect(dialog.getByRole('alert')).toContainText('连接配置有未保存的修改');
    expect(syncRequests).toHaveLength(failedSaveSyncCount);
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    // 等待保存时修改的 Key 属于下一份草稿，旧响应不能将它清空。
    saveOK = true; holdSave = true;
    await page.getByTestId('ai-save-settings').click();
    await expect.poll(() => Boolean(releaseSave), { intervals: [10] }).toBe(true);
    await apiKey.fill('fixture-newer-draft-key');
    releaseSave!(); holdSave = false;
    await expect(page.getByTestId('ai-save-settings')).toBeEnabled();
    await expect(apiKey).toHaveValue('fixture-newer-draft-key');
    await page.getByTestId('ai-online-models').click();
    await expect(dialog.getByRole('alert')).toContainText('连接配置有未保存的修改');
    expect(syncRequests).toHaveLength(failedSaveSyncCount);
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await page.getByTestId('ai-save-settings').click();
    await expect(apiKey).toHaveValue('');
    expect(saveRequests.at(-1).llm.apiKey).toBe('fixture-newer-draft-key');
    await page.reload();
    await settleForm();
    await expect(page.getByTestId('ai-base-url')).toHaveValue('');
    await expect(page.getByTestId('ai-default-base-url-hint')).toContainText('https://openai.example.invalid/v1');
    await page.getByTestId('ai-base-url').click();
    await expect(page.getByTestId('ai-base-url')).toBeFocused();
    await page.screenshot({ path: info.outputPath(`cleared-base-url-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: !narrow });
    expect(generations).toHaveLength(0); expect(errors).toEqual([]);
  });
}
