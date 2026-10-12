-- dev 历史空间 Profile 绑定修复：仅使用已复核的同租户 published v1 对象。
-- 重复执行不改写已正确的绑定；任何不符合预期的映射、策略或发布状态明确失败。
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '20s';
DO $repair$
DECLARE
    current_space "public".knowledge_spaces%ROWTYPE;
BEGIN
    SELECT * INTO STRICT current_space FROM "public".knowledge_spaces
    WHERE uuid = '30a6d654-c4c5-4a33-9806-6d66c6728187'::uuid AND tenant_uuid = '6b5d0240-9920-46da-b707-88200e0f51ea' AND deleted_at IS NULL FOR UPDATE;
    IF current_space.space_name <> 'dev'
       OR current_space.status <> 'pending_iam'
       OR current_space.ingestion_profile_key <> 'p1_general'
       OR current_space.index_profile_key <> 'p1_general'
       OR current_space.rag_profile_key <> 'p1_general'
       OR NOT current_space.feature_flags @> '["rag.strategy_package:h_fusion","rag.scene:product_specs"]'::jsonb THEN
        RAISE EXCEPTION 'dev space identity, state, profile keys or strategy/scene changed; repair refused';
    END IF;
    IF (current_space.ingestion_profile_uuid IS NOT NULL AND current_space.ingestion_profile_uuid <> '4245b851-0710-4566-9f53-ecf3dd7e04cc'::uuid)
       OR (current_space.index_profile_uuid IS NOT NULL AND current_space.index_profile_uuid <> '81e39770-93dd-4903-918c-9847224d30a4'::uuid)
       OR (current_space.rag_profile_uuid IS NOT NULL AND current_space.rag_profile_uuid <> 'aa8c8f9c-6558-435d-a352-4727adb8163f'::uuid) THEN
        RAISE EXCEPTION 'dev profile UUID mapping changed; repair refused';
    END IF;
    PERFORM 1 FROM "public".knowledge_ingestion_profile_versions
    WHERE uuid = '4245b851-0710-4566-9f53-ecf3dd7e04cc'::uuid AND tenant_uuid = '6b5d0240-9920-46da-b707-88200e0f51ea'
      AND profile_key = 'p1_general' AND version = 1 AND status = 'published' AND deleted_at IS NULL FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION 'exact published ingestion Profile v1 unavailable'; END IF;
    PERFORM 1 FROM "public".knowledge_index_profile_versions
    WHERE uuid = '81e39770-93dd-4903-918c-9847224d30a4'::uuid AND tenant_uuid = '6b5d0240-9920-46da-b707-88200e0f51ea'
      AND profile_key = 'p1_general' AND version = 1 AND status = 'published' AND deleted_at IS NULL FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION 'exact published index Profile v1 unavailable'; END IF;
    PERFORM 1 FROM "public".knowledge_rag_profile_versions
    WHERE uuid = 'aa8c8f9c-6558-435d-a352-4727adb8163f'::uuid AND tenant_uuid = '6b5d0240-9920-46da-b707-88200e0f51ea'
      AND profile_key = 'p1_general' AND version = 1 AND status = 'published' AND deleted_at IS NULL FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION 'exact published rag Profile v1 unavailable'; END IF;
    UPDATE "public".knowledge_spaces SET
      ingestion_profile_uuid = '4245b851-0710-4566-9f53-ecf3dd7e04cc'::uuid,
      index_profile_uuid = '81e39770-93dd-4903-918c-9847224d30a4'::uuid,
      rag_profile_uuid = 'aa8c8f9c-6558-435d-a352-4727adb8163f'::uuid,
      updated_at = CURRENT_TIMESTAMP
    WHERE uuid = '30a6d654-c4c5-4a33-9806-6d66c6728187'::uuid AND tenant_uuid = '6b5d0240-9920-46da-b707-88200e0f51ea' AND deleted_at IS NULL
      AND (ingestion_profile_uuid IS NULL OR index_profile_uuid IS NULL OR rag_profile_uuid IS NULL);
END
$repair$;
COMMIT;
