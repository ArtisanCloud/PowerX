package storagegovrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
)

// DatasetRelationIdentity is resolved from a trusted cluster binding and
// PostgreSQL catalogs. It is independent of plugin and tenant aliases.
type DatasetRelationIdentity struct {
	ClusterID  string
	DatabaseID int64
	RelationID int64
}

// DatasetRelationSample carries read-only catalog metrics.
type DatasetRelationSample struct {
	Identity DatasetRelationIdentity
	Bytes    int64
}

// ObserveDatasetRelations measures exact admitted members of a single trusted
// schema. The caller supplies a connection from the verified source binding.
func ObserveDatasetRelations(ctx context.Context, db *gorm.DB, clusterID, schema string, names []string) ([]DatasetRelationSample, error) {
	return observeDatasetRelations(ctx, db, clusterID, schema, "", names, false)
}

// ObserveBoundDatasetRelations verifies the installed role can read every
// member. Core uses its existing pool only for the catalog query.
func ObserveBoundDatasetRelations(ctx context.Context, db *gorm.DB, clusterID, schema, role string, names []string) ([]DatasetRelationSample, error) {
	if role == "" {
		return nil, errors.New("dataset role unavailable")
	}
	return observeDatasetRelations(ctx, db, clusterID, schema, role, names, false)
}

// ObserveBoundDatasetRelationsTx performs the same catalog-only select on an
// already pinned Core transaction without borrowing a second pool connection.
func ObserveBoundDatasetRelationsTx(ctx context.Context, tx *gorm.DB, clusterID, schema, role string, names []string) ([]DatasetRelationSample, error) {
	if role == "" {
		return nil, errors.New("dataset role unavailable")
	}
	return observeDatasetRelations(ctx, tx, clusterID, schema, role, names, true)
}

func observeDatasetRelations(ctx context.Context, db *gorm.DB, clusterID, schema, role string, names []string, inTx bool) ([]DatasetRelationSample, error) {
	if db == nil || db.Dialector.Name() != "postgres" || clusterID == "" || schema == "" || len(names) < 1 || len(names) > 32 {
		return nil, errors.New("dataset metric source unavailable")
	}
	run, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var rows []struct {
		DatabaseID int64
		RelationID int64
		Bytes      int64
	}
	selectCatalog := func(tx *gorm.DB) error {
		return tx.WithContext(run).Raw(`SELECT d.oid::bigint AS database_id, c.oid::bigint AS relation_id,
			pg_catalog.pg_total_relation_size(c.oid) AS bytes
			FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
			JOIN pg_catalog.pg_database d ON d.datname = current_database()
			WHERE n.nspname = ? AND c.relname IN ? AND c.relkind IN ('r','m')
			AND NOT c.relispartition
			AND CASE WHEN ? = '' THEN pg_catalog.has_table_privilege(c.oid, 'SELECT')
				ELSE pg_catalog.has_schema_privilege(?, n.oid, 'USAGE')
					AND pg_catalog.has_table_privilege(?, c.oid, 'SELECT') END`, schema, names, role, role, role).Scan(&rows).Error
	}
	var err error
	if inTx {
		err = selectCatalog(db)
	} else {
		err = db.WithContext(run).Transaction(selectCatalog, &sql.TxOptions{ReadOnly: true})
	}
	if err != nil {
		return nil, err
	}
	if len(rows) != len(names) {
		return nil, errors.New("dataset relation coverage incomplete")
	}
	samples := make([]DatasetRelationSample, 0, len(rows))
	for _, row := range rows {
		samples = append(samples, DatasetRelationSample{Identity: DatasetRelationIdentity{clusterID, row.DatabaseID, row.RelationID}, Bytes: row.Bytes})
	}
	return samples, nil
}

// SumDatasetPhysicalBytes counts each physical object only once. Incomplete
// identities or contradictory samples must remain unknown in the directory.
func SumDatasetPhysicalBytes(samples []DatasetRelationSample) (int64, error) {
	const maxSafeBytes = 1<<53 - 1
	if len(samples) == 0 {
		return 0, errors.New("dataset relation coverage unknown")
	}
	seen := make(map[DatasetRelationIdentity]int64, len(samples))
	var total int64
	for _, sample := range samples {
		if sample.Identity.ClusterID == "" || sample.Identity.DatabaseID <= 0 || sample.Identity.RelationID <= 0 ||
			sample.Bytes < 0 || sample.Bytes > maxSafeBytes {
			return 0, errors.New("invalid dataset relation sample")
		}
		if previous, ok := seen[sample.Identity]; ok {
			if previous != sample.Bytes {
				return 0, errors.New("dataset relation samples disagree")
			}
			continue
		}
		if sample.Bytes > maxSafeBytes-total {
			return 0, errors.New("dataset physical size out of range")
		}
		seen[sample.Identity] = sample.Bytes
		total += sample.Bytes
	}
	return total, nil
}
