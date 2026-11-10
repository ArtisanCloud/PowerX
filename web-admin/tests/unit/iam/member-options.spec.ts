import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useMemberService } from '~/composables/api/services/memberService';
const client = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('~/composables/api/index', () => ({ useApiClient: () => client }));
beforeEach(() => vi.clearAllMocks());
const member = (id: number) => ({
  Member: {
    id,
    username: `user${id}`,
    display_name: `Member ${id}`,
    status: 1,
  },
  User: { email: `user${id}@example.invalid` },
  DeptIDs: [7],
});

describe('department leader candidates', () => {
  it('reads every formal page and resolves nested member IDs and labels', async () => {
    client.get
      .mockResolvedValueOnce({
        data: { items: [member(1)], pagination: { pages: 2 } },
      })
      .mockResolvedValueOnce({
        data: { items: [member(101)], pagination: { pages: 2 } },
      });
    const rows = await useMemberService().listAll({ status: 1 });
    expect(client.get).toHaveBeenNthCalledWith(1, '/admin/iam/members', {
      params: { status: 1, page: 1, page_size: 100 },
    });
    expect(client.get).toHaveBeenNthCalledWith(2, '/admin/iam/members', {
      params: { status: 1, page: 2, page_size: 100 },
    });
    expect(rows).toMatchObject([
      { id: 1, name: 'Member 1' },
      { id: 101, name: 'Member 101' },
    ]);
    expect(rows[0].Member.id).toBe(1);
  });
  it('returns an actual empty directory', async () => {
    client.get.mockResolvedValue({
      data: { items: [], pagination: { pages: 0 } },
    });
    await expect(useMemberService().listAll()).resolves.toEqual([]);
  });
  it('does not turn a failed later page into an empty or partial list', async () => {
    client.get
      .mockResolvedValueOnce({
        data: { items: [member(1)], pagination: { pages: 2 } },
      })
      .mockRejectedValueOnce(new Error('permission denied'));
    await expect(useMemberService().listAll()).rejects.toThrow(
      'permission denied',
    );
  });
  it.each([
    { items: [member(1)] },
    { items: [{ Member: { uuid: 'missing-id' } }], pagination: { pages: 1 } },
  ])('rejects a malformed directory response', async (data) => {
    client.get.mockResolvedValue({ data });
    await expect(useMemberService().listAll()).rejects.toThrow(
      'IAM_MEMBER_LIST_RESPONSE_INVALID',
    );
  });
});
