# Feature Specification: Media Asset Admin Capabilities

**Feature Branch**: `001-media-storage`  
**Created**: 2025-10-07  
**Status**: Draft  
**Input**: User description: "Refer to docs/media_storage_design.md docs/media_storage_admin_api.md"

## User Scenarios & Testing (mandatory)

### Primary User Story

As an ops/content admin, I want to upload, search, update, and retire media assets within a single admin console, and generate temporary access links for business teams, so that images and videos can be managed efficiently.

### Acceptance Scenarios

1. **Given** the admin has console access and storage drivers are configured, **When** the admin uploads a local file or provides an external file URL, **Then** the system validates the request, persists media metadata, and returns an accessible resource payload.
2. **Given** the admin needs to locate historical assets, **When** the admin filters the media list by keyword, storage driver, or owner subject, **Then** the system returns a paginated result set with total count and allows viewing full details.
3. **Given** the admin uses the Web Admin console, **When** the admin completes a presigned upload flow (create → presign → upload → refresh), **Then** the console shows the updated asset (including size/mime) and supports preview/download via controlled resource access.

### Edge Cases

- If a creation request contains neither a file nor a valid external link, the system must reject the operation and prompt for a usable resource.
- If the admin tries to access a soft-deleted or disabled asset, the system must block access and indicate that the resource is unavailable.
- Upload size limits defer to the configured storage driver; the admin should receive a clear error if the driver rejects an oversized payload.

## Requirements (mandatory)

### Functional Requirements

- **FR-001**: Allow authorized admins to create media assets via local upload or trusted external link; record name, driver, folder, owner subject, and optional tags, and surface driver-originated errors (including size limits) back to the admin.
- **FR-002**: Validate that the selected storage driver is enabled; if missing or disabled, reject the operation with a clear reason.
- **FR-003**: Provide a paginated media list supporting filters by keyword, driver type, owner subject (type & ID), and optional tags; expose total count.
- **FR-004**: Provide an asset details view with base metadata, created/updated time, business status, and a driver-specific access URL (e.g., presigned URL with expiry).
- **FR-005**: Allow updating business attributes (name, description, tags, business status set to Draft / Under Review / Published / Archived) without altering the underlying storage location or driver configuration.
- **FR-006**: Support soft deletion and record operator/time; by default soft delete immediately and hand off the asset to a scheduled cleanup job for physical removal, with policy-based overrides when explicit immediate deletion is required.
- **FR-007**: Generate time-limited presigned links for an existing or to-be-uploaded resource; links must expire automatically after 12 hours by default and only authorized admins may generate them.
- **FR-008**: Persist auditable trails for upload/update/delete/presign operations to trace actor, source, and parameters.

### Console UI Requirements (Web Admin)

> UI 设计与页面流详见：`docs/plan/content/media.md`

- **FR-UI-001**: Provide a “Media Library” entry in the admin console for tenant-scoped media asset management (list / detail / edit / delete).
- **FR-UI-002**: Support search and filtering in the console by keyword, tags (AND semantics), business status, storage driver, and recycle-bin mode (only deleted / include deleted).
- **FR-UI-003**: Support presigned upload in the console as the primary upload path (create asset → presign → upload → refresh), including progress, retry, and clear failure messages.
- **FR-UI-004**: Support external-link ingestion in the console (register an asset by URL and preview/open it safely).
- **FR-UI-005**: Provide safe preview/download behaviors by default (prefer controlled, authenticated resource access; avoid accidental exposure through public anonymous endpoints).
- **FR-UI-006**: Provide basic batch operations in the console (minimum: batch soft-delete; optional: batch tag edits).

### Key Entities

- **Media Asset**: A single file or external resource managed by the platform, including name, storage driver, access URL, size, owner subject, tags, business status (Draft / Under Review / Published / Archived), timestamps, and soft-delete marker.
- **Storage Driver**: Available storage options (local filesystem or S3-compatible object storage), including driver identifier, availability, base access path, and presign configuration.
- **Presign Request**: A temporary authorization for upload/download, including target asset ID, link type (upload/download), expiry, allowed HTTP method, and extra fields for the frontend.

## Assumptions & Dependencies

- Console authentication/authorization exists and restricts API access to identified actors.
- Base storage configuration (bucket, credentials, base URL) is maintained by ops/config center; the media module relies on it.
- Unified tagging and owner subject standards exist; asset creation/update must comply with them.
- An operations-maintained scheduled task will process soft-deleted assets for physical removal according to retention policies.
- Maximum upload size is governed by each storage driver; the admin console will not impose an additional global cap.
- The Web Admin console can resolve tenant context and attach it to requests consistently (tenant isolation is mandatory).

## Clarifications

### Session 2025-10-07

- Q: 媒体资产的“业务状态”需要明确枚举以便建模和验收，请选择最符合预期的状态集合。 → A: 草稿 / 审核中 / 已发布 / 已归档
- Q: 预签名链接的默认有效期需要设定明确目标，以便验证和配置，请选择最合适的选项。 → A: 12 小时
- Q: 当管理员执行删除操作时，如果对象存储中的文件没有额外策略约束，默认的删除策略应该是什么？ → A: 先软删除，待后台定时任务物理清理
- Q: 请确认后台上传单个媒体文件的大小上限，用于限制和容量规划。 → A: 不限制，由驱动自行约束

## Review & Acceptance Checklist

### Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

### Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Execution Status

- [x] User description parsed
- [x] Key concepts extracted
- [x] Ambiguities addressed
- [x] User scenarios defined
- [x] Requirements generated
- [x] Entities identified
- [x] Review checklist passed
