package migration

import (
	"fmt"

	"gorm.io/gorm"
)

// CreatePluginReleasePartitions configures optional partitioning helpers for plugin release candidates.
// Actual conversion to partitioned tables requires manual DBA action; this migration only installs helpers.
func CreatePluginReleasePartitions(db *gorm.DB) error {
	if err := db.Exec(`CREATE SCHEMA IF NOT EXISTS plugin_release;`).Error; err != nil {
		return fmt.Errorf("ensure plugin_release schema failed: %w", err)
	}

	const partitionedCheck = `
SELECT EXISTS (
	SELECT 1 FROM pg_partitioned_table WHERE partrelid = 'plugin_release_candidates'::regclass
);`
	type result struct {
		Exists bool
	}
	var r result
	if err := db.Raw(partitionedCheck).Scan(&r).Error; err != nil {
		return err
	}

	if r.Exists {
		const partitionSQL = `
DO $$
DECLARE
    start_month DATE := date_trunc('month', CURRENT_DATE) - interval '2 month';
    end_month DATE := date_trunc('month', CURRENT_DATE) + interval '6 month';
    cur_month DATE := start_month;
    partition_name TEXT;
    partition_start DATE;
    partition_end DATE;
BEGIN
    WHILE cur_month <= end_month LOOP
        partition_name := format('plugin_release_candidates_%s', to_char(cur_month, 'YYYYMM'));
        partition_start := date_trunc('month', cur_month);
        partition_end := partition_start + interval '1 month';

        IF NOT EXISTS (
            SELECT 1
            FROM   pg_class c
            JOIN   pg_namespace n ON n.oid = c.relnamespace
            WHERE  n.nspname = 'plugin_release'
            AND    c.relname = partition_name
        ) THEN
            EXECUTE format(
                'CREATE TABLE plugin_release.%s PARTITION OF plugin_release_candidates FOR VALUES FROM (%L) TO (%L);',
                partition_name,
                partition_start,
                partition_end
            );
        END IF;

        cur_month := cur_month + interval '1 month';
    END LOOP;
END
$$;`
		if err := db.Exec(partitionSQL).Error; err != nil {
			return fmt.Errorf("create plugin_release partitions failed: %w", err)
		}
	}

	const retentionSQL = `
CREATE OR REPLACE FUNCTION plugin_release.cleanup_candidate_partitions(retention_months integer)
RETURNS void AS $$
DECLARE
    cutoff DATE := date_trunc('month', CURRENT_DATE) - (retention_months || ' month')::interval;
    part RECORD;
BEGIN
    FOR part IN
        SELECT inhrelid::regclass AS partition_table
        FROM pg_inherits
        JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
        WHERE parent.relname = 'plugin_release_candidates'
    LOOP
        IF (SELECT pg_catalog.pg_get_expr(relpartbound, inhrelid, true) < format('FOR VALUES FROM (%s)', cutoff)) THEN
            EXECUTE format('DROP TABLE IF EXISTS %s CASCADE;', part.partition_table);
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;`

	return db.Exec(retentionSQL).Error
}
