import { defineComponent, h } from "vue";
import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
const auth=vi.hoisted(()=>({session:null as any}));
vi.mock('~/utils/auth-session',()=>({AUTH_SESSION_EVENT:'powerx:auth-session-changed',readAuthSession:()=>auth.session}));
const transport = vi.hoisted(() => ({ connect: vi.fn(), close: vi.fn(), stream: vi.fn() }));
vi.mock("~/utils/backup-sse", () => ({ connectBackupEvents: transport.connect }));
vi.mock("~/composables/api", () => ({ useApiClient: () => ({ stream: transport.stream }) }));
import { useBackupEvents, type BackupResource } from "~/composables/useBackupEvents";
const wrappers: ReturnType<typeof mount>[] = [];
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.clearAllMocks();auth.session=null; });
describe("shared backup notifications", () => {
  it("shares a single stream across sections and releases it after the last subscriber leaves", () => {
    transport.connect.mockReturnValue(transport.close);
    const snapshots = vi.fn(), archives = vi.fn();
    const panel = (resource: BackupResource, change: () => void) => {
      const wrapper = mount(defineComponent({ setup() { useBackupEvents(resource, change); return () => h('div'); } }));
      wrappers.push(wrapper); return wrapper;
    };
    const first = panel("snapshots", snapshots), next = panel("archives", archives);
    expect(transport.connect).toHaveBeenCalledTimes(1);
    const receive = transport.connect.mock.calls[0][1];
    receive({ name: "invalidate", data: { resources: ["snapshots"] } });
    expect(snapshots).toHaveBeenCalledTimes(1); expect(archives).not.toHaveBeenCalled();
    first.unmount(); expect(transport.close).not.toHaveBeenCalled();
    receive({ name: "invalidate", data: { resources: ["snapshots", "archives"] } });
    expect(snapshots).toHaveBeenCalledTimes(1); expect(archives).toHaveBeenCalledTimes(1);
    next.unmount(); expect(transport.close).toHaveBeenCalledTimes(1);
  });
  it('replaces the shared stream on account change without reconnecting on token refresh',()=>{
    auth.session={sessionId:'A',tenantUuid:'tenant',status:'active'};transport.connect.mockReturnValue(transport.close);
    const change=vi.fn();const w=mount(defineComponent({setup(){useBackupEvents('snapshots',change);return()=>h('div')}}));wrappers.push(w);
    expect(transport.connect).toHaveBeenCalledTimes(1);
    auth.session={...auth.session,revision:2};window.dispatchEvent(new CustomEvent('powerx:auth-session-changed'));expect(transport.connect).toHaveBeenCalledTimes(1);
    auth.session={sessionId:'B',tenantUuid:'tenant',status:'active'};window.dispatchEvent(new CustomEvent('powerx:auth-session-changed'));
    expect(transport.close).toHaveBeenCalledTimes(1);expect(transport.connect).toHaveBeenCalledTimes(2);expect(change).toHaveBeenCalledTimes(1);
    auth.session={sessionId:'signed-out',status:'signed-out'};window.dispatchEvent(new CustomEvent('powerx:auth-session-changed'));expect(transport.close).toHaveBeenCalledTimes(2);expect(transport.connect).toHaveBeenCalledTimes(2);
  });

});
