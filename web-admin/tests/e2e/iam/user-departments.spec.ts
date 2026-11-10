import { test, expect, type Page } from '@playwright/test';
test.use({ actionTimeout: 10_000 });
test.setTimeout(120_000);
const tenant = '00000000-0000-4000-8000-000000000001';
async function prepare(page: Page, root = false) {
  const departments = [
    { id: 1, name: '人力资源部', key: 'hr', status: 1 },
    {
      id: 2,
      name: '研发部',
      key: 'engineering',
      status: 1,
      children: [
        { id: 3, name: '平台组', key: 'platform', parent_id: 2, status: 1 },
      ],
    },
  ];
  const records = Array.from({ length: 101 }, (_, i) => ({
    Member: {
      id: i + 1,
      uuid: `00000000-0000-4000-8000-${String(i + 100).padStart(12, '0')}`,
      username: `person${i + 1}`,
      display_name: i === 100 ? '末页负责人' : `Person ${i + 1}`,
      status: 1,
      avatar_url: '/favicon.ico',
    },
    User: {
      uuid: `00000000-0000-4000-8000-${String(i + 300).padStart(12, '0')}`,
      display_name: `Person ${i + 1}`,
      email: `person${i + 1}@example.invalid`,
      status: 1,
      is_root: false,
    },
    DeptIDs: i === 0 ? [1, 2] : i === 1 ? [3] : [],
  }));
  const queries: Record<string, string>[] = [];
  const updates: any[] = [];
  const leaderUpdates: any[] = [];
  const candidatePages: string[] = [];
  const errors: string[] = [];
  let failCandidates = false;
  page.on('pageerror', (error) => errors.push(error.message));
  await page.addInitScript(
    ({ tenant }) => {
      const token = `eyJhbGciOiJub25lIn0.${btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 3600, tid: tenant, sub: 'department-test' }))}.test`;
      localStorage.setItem(
        'powerx.session',
        JSON.stringify({
          version: 1,
          status: 'active',
          sessionId: 'department-test',
          revision: 1,
          accessToken: token,
          refreshToken: 'test-refresh',
          tokenType: 'Bearer',
          expiresAt: Date.now() + 3600000,
          scope: 'access',
          tenantUuid: tenant,
        }),
      );
      localStorage.setItem('px_locale', 'zh');
      localStorage.setItem('powerx-color-mode', 'dark');
      localStorage.setItem('welcome_guide_completed', 'true');
    },
    { tenant },
  );
  await page.routeWebSocket(/\/api\/ws/, (socket) => socket.close());
  await page.route(
    (url) => url.pathname.startsWith('/api/'),
    async (route) => {
      const req = route.request();
      const url = new URL(req.url());
      const path = url.pathname;
      if (path.includes('/_nuxt_icon/')) {
        await route.fallback();
        return;
      }
      let data: unknown = {};
      if (path.endsWith('/setup/status'))
        data = {
          configured: true,
          requires_login: true,
          install_status: 'installed',
        };
      else if (path.endsWith('/me/context'))
        data = {
          is_root: root,
          current_tenant_uuid: tenant,
          current_member_id: 1,
          current_member_uuid: records[0].Member.uuid,
          user: {
            id: 1,
            uuid: '00000000-0000-4000-8000-000000000003',
            display_name: 'Department Tester',
            email: 'tester@example.invalid',
            status: 1,
          },
          members: [
            {
              tenant_uuid: tenant,
              tenant_name: 'Test Tenant',
              member_id: 1,
              is_admin: true,
            },
          ],
        };
      else if (/\/(?:system\/users|iam\/members)\/[^/]+\/roles$/.test(path))
        data = { role_uuids: ['role-user'] };
      else if (path.endsWith('/iam/roles'))
        data = {
          items: [
            { uuid: 'role-user', code: 'role_user', name: 'Tenant User' },
          ],
          pagination: { pages: 1, total: 1, page: 1, page_size: 200 },
        };
      else if (path.endsWith('/departments/tree')) data = departments;
      else if (/\/departments\/\d+$/.test(path)) {
        const id = Number(path.split('/').at(-1));
        const department = departments.find((d) => d.id === id)!;
        if (req.method() === 'PATCH') {
          const body = req.postDataJSON();
          leaderUpdates.push(body);
          Object.assign(department, body);
        }
        data = department;
      } else if (
        /\/(?:system\/users|iam\/members)\/[^/]+$/.test(path) &&
        req.method() === 'PATCH'
      ) {
        const body = req.postDataJSON();
        updates.push(body);
        const values = body.member || body;
        Object.assign(records[0].Member, values);
        if ('dept_ids' in body) records[0].DeptIDs = body.dept_ids;
        data = { ok: true };
      } else if (
        path.endsWith('/iam/members') ||
        path.endsWith('/system/users')
      ) {
        const params = Object.fromEntries(url.searchParams);
        const candidate =
          path.endsWith('/iam/members') && params.page_size === '100';
        if (candidate) {
          candidatePages.push(params.page);
          if (failCandidates) {
            await route.fulfill({
              status: 503,
              contentType: 'application/json',
              body: JSON.stringify({
                code: 503,
                message: 'fixture candidate failure',
              }),
            });
            return;
          }
        } else queries.push(params);
        const dept = Number(params.dept_id);
        const list = dept
          ? records.filter((r) =>
              r.DeptIDs.some((id) => id === dept || (dept === 2 && id === 3)),
            )
          : records;
        const pageNum = Number(params.page || 1),
          size = Number(params.page_size || 10);
        data = {
          items: list.slice((pageNum - 1) * size, pageNum * size),
          pagination: {
            total: list.length,
            page: pageNum,
            page_size: size,
            pages: Math.ceil(list.length / size),
          },
        };
      } else if (path.endsWith('/menus'))
        data = {
          menus: [
            {
              id: 'users',
              title: 'Users',
              path: '/settings/users',
              visible: true,
              origin: 'system',
            },
          ],
        };
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, message: 'success', data }),
      });
    },
  );
  await page.goto('/settings/users');
  await expect(
    page.locator('main').getByRole('button', { name: '用户', exact: true }),
  ).toBeVisible({ timeout: 60_000 });
  if ((page.viewportSize()?.width || 1440) < 768)
    await page
      .getByRole('complementary')
      .getByRole('button', { name: '收起', exact: true })
      .first()
      .click();
  await page
    .locator('main')
    .getByRole('button', { name: '用户', exact: true })
    .click();
  await expect(page.getByTestId('user-pagination')).toContainText('共 101 条');
  return {
    queries,
    updates,
    leaderUpdates,
    candidatePages,
    errors,
    setCandidateFailure: (value: boolean) => {
      failCandidates = value;
    },
  };
}

for (const viewport of [
  { name: 'desktop', width: 1440, height: 1000, root: false },
  { name: 'narrow-root', width: 390, height: 844, root: true },
]) {
  test(`${viewport.name}: department filter, edit and leader selection`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    const state = await prepare(page, viewport.root);
    // 部门筛选由服务端执行，包含子部门；重置回到完整列表。
    await page.getByRole('button', { name: '选择部门', exact: true }).click();
    await page
      .getByRole('treeitem')
      .filter({ hasText: '研发部' })
      .first()
      .click();
    await expect(page.getByTestId('user-pagination')).toContainText('共 2 条');
    expect(state.queries.at(-1)).toMatchObject({
      dept_id: '2',
      recursive: 'true',
      page: '1',
    });
    await page.getByRole('button', { name: '重置筛选', exact: true }).click();
    await expect(page.getByTestId('user-pagination')).toContainText(
      '共 101 条',
    );
    expect(state.queries.at(-1)).not.toHaveProperty('dept_id');

    const firstRow = page
      .getByRole('row')
      .filter({
        has: page.getByRole('cell', { name: 'person1', exact: true }),
      });
    await firstRow.getByRole('button', { name: '编辑', exact: true }).click();
    let dialog = page.getByRole('dialog');
    const selected = page.getByTestId('user-departments');
    await expect(selected).toContainText('人力资源部');
    await expect(selected).toContainText('研发部');
    await selected.click();
    await page.getByRole('option', { name: '人力资源部', exact: true }).click();
    await page.keyboard.press('Escape');
    await dialog.getByRole('button', { name: '保存', exact: true }).click();
    await expect(dialog).toBeHidden();
    expect(state.updates.at(-1).dept_ids).toEqual([2]);
    await firstRow.getByRole('button', { name: '编辑', exact: true }).click();
    await expect(selected).toContainText('研发部');
    await selected.click();
    await page.getByRole('option', { name: '研发部', exact: true }).click();
    await page.keyboard.press('Escape');
    await dialog.getByRole('button', { name: '保存', exact: true }).click();
    await expect(dialog).toBeHidden();
    expect(state.updates.at(-1).dept_ids).toEqual([]);

    await page
      .locator('main')
      .getByRole('button', { name: '部门', exact: true })
      .click();
    await page
      .getByRole('treeitem')
      .filter({ hasText: '人力资源部' })
      .getByRole('button')
      .first()
      .click();
    dialog = page.getByRole('dialog');
    const leader = page.getByTestId('department-leader');
    await expect(leader).toBeEnabled();
    expect(state.candidatePages).toContain('2');
    await leader.click();
    await page.getByPlaceholder('搜索负责人').fill('末页');
    await page.getByRole('option', { name: '末页负责人', exact: true }).click();
    await expect(leader).toContainText('末页负责人');
    await page.screenshot({
      path: testInfo.outputPath(`departments-${viewport.name}.png`),
      animations: 'disabled',
    });
    const box = await dialog.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width);
    await dialog.getByRole('button', { name: '保存', exact: true }).click();
    await expect(dialog).toBeHidden();
    expect(state.leaderUpdates.at(-1)).toMatchObject({ leader_member_id: 101 });
    await page
      .getByRole('treeitem')
      .filter({ hasText: '人力资源部' })
      .getByRole('button')
      .first()
      .click();
    await leader.click();
    await page.getByRole('option', { name: '无', exact: true }).click();
    await dialog.getByRole('button', { name: '保存', exact: true }).click();
    await expect(dialog).toBeHidden();
    expect(state.leaderUpdates.at(-1)).toMatchObject({
      leader_member_id: null,
    });

    state.setCandidateFailure(true);
    await page
      .getByRole('treeitem')
      .filter({ hasText: '人力资源部' })
      .getByRole('button')
      .first()
      .click();
    await expect(dialog.getByRole('alert')).toContainText('负责人列表加载失败');
    await expect(leader).toBeDisabled();
    state.setCandidateFailure(false);
    await dialog.getByRole('button', { name: '重试', exact: true }).click();
    await expect(leader).toBeEnabled();
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    expect(state.errors).toEqual([]);
  });
}
