export interface MaintenanceIntentIdentity { sessionId: string; tenantUuid: string }
export interface MaintenanceIntent extends MaintenanceIntentIdentity {
  version: 1; scopeId: string; requestId: string; previewId: string; exclusiveLockConfirmed: boolean; at: number;
}
type Store = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;
const uuid = (x: unknown) => typeof x === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(x);
const key = (id: MaintenanceIntentIdentity) => 'powerx.database-maintenance.intent.v1.' + encodeURIComponent(id.sessionId) + '.' + encodeURIComponent(id.tenantUuid);
function valid(value: any, identity: MaintenanceIntentIdentity): value is MaintenanceIntent {
  return value?.version === 1 && value.sessionId === identity.sessionId && value.tenantUuid === identity.tenantUuid &&
    typeof value.scopeId === 'string' && /^[0-9a-f]{64}$/.test(value.scopeId) && uuid(value.requestId) && uuid(value.previewId) &&
    typeof value.exclusiveLockConfirmed === 'boolean' && Number.isSafeInteger(value.at) && value.at > 0;
}
export function readMaintenanceIntent(storage: Store, identity: MaintenanceIntentIdentity): MaintenanceIntent | undefined {
  const raw = storage.getItem(key(identity)); if (!raw) return;
  if (raw.length > 4096) throw new Error('DATABASE_MAINTENANCE_INTENT_INVALID');
  let value: unknown; try { value = JSON.parse(raw); } catch { throw new Error('DATABASE_MAINTENANCE_INTENT_INVALID'); }
  if (!valid(value, identity)) throw new Error('DATABASE_MAINTENANCE_INTENT_INVALID');
  return value;
}
export function saveMaintenanceIntent(storage: Store, value: MaintenanceIntent): void {
  if (!valid(value, value)) throw new Error('DATABASE_MAINTENANCE_INTENT_INVALID');
  const old = readMaintenanceIntent(storage, value);
  if (old && (old.requestId !== value.requestId || old.previewId !== value.previewId || old.scopeId !== value.scopeId)) throw new Error('DATABASE_MAINTENANCE_RESULT_UNCONFIRMED');
  storage.setItem(key(value), JSON.stringify(value));
}
export function clearMaintenanceIntent(storage: Store, identity: MaintenanceIntentIdentity, requestID: string): void {
  const old = readMaintenanceIntent(storage, identity);
  if (old?.requestId === requestID) storage.removeItem(key(identity));
}
