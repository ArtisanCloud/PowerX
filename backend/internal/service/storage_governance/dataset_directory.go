package storagegov

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/google/uuid"
)

type DatasetFilter struct {
	OwnerKind, PluginID, CapacityLevel, AvailableAction string
	Limit                                               int
	Cursor                                              string
}

type DatasetObservation struct {
	State          string     `json:"state"`
	Level          string     `json:"level"`
	Coverage       string     `json:"coverage"`
	ObservedAt     *time.Time `json:"observed_at"`
	TotalBytes     *int64     `json:"total_bytes"`
	NetChangeBytes *int64     `json:"net_change_bytes"`
}

type DatasetPolicyReference struct {
	Kind          string `json:"kind"`
	LegacyTableID string `json:"legacy_table_id"`
}

type DatasetView struct {
	DatasetUUID        uuid.UUID                `json:"dataset_uuid"`
	DatasetKey         string                   `json:"dataset_key"`
	Name               string                   `json:"name"`
	OwnerKind          string                   `json:"owner_kind"`
	PluginID           string                   `json:"plugin_id,omitempty"`
	TenantScope        string                   `json:"tenant_scope"`
	SourceBindingUUID  *uuid.UUID               `json:"source_binding_uuid,omitempty"`
	SourceState        string                   `json:"source_state"`
	RegistrationState  string                   `json:"registration_state"`
	AcceptanceRevision int64                    `json:"acceptance_revision,omitempty"`
	ContractVersion    int64                    `json:"contract_version,omitempty"`
	StatusReason       string                   `json:"status_reason,omitempty"`
	GovernanceClass    string                   `json:"governance_class"`
	Observation        DatasetObservation       `json:"observation"`
	AvailableActions   []string                 `json:"available_actions"`
	RetentionBounds    *DatasetRetentionBounds  `json:"retention_bounds,omitempty"`
	ExistingPolicyRefs []DatasetPolicyReference `json:"existing_policy_refs,omitempty"`
}

type DatasetPage struct {
	Items             []DatasetView `json:"items"`
	NextCursor        *string       `json:"next_cursor"`
	PagePhysicalBytes *int64        `json:"page_physical_bytes"`
}

type datasetCursor struct {
	Scope string
	After string
}

func (s *Service) ListDatasets(ctx context.Context, filter DatasetFilter) (DatasetPage, error) {
	out := DatasetPage{Items: []DatasetView{}}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit < 1 || filter.Limit > 100 || len(filter.Cursor) > 1024 ||
		!datasetFilterValue(filter.OwnerKind, "system", "plugin") ||
		!datasetFilterValue(filter.CapacityLevel, "normal", "warning", "unknown") ||
		!datasetFilterValue(filter.AvailableAction, "observe", "preview", "delete", "archive", "open_existing_policy") ||
		(filter.PluginID != "" && !idPattern.MatchString(filter.PluginID)) {
		return out, controlError(400, "DATASET_FILTER_INVALID")
	}
	scope := controlDigest([]byte(strings.Join([]string{s.opts.Identity.InstanceID, filter.OwnerKind, filter.PluginID, filter.CapacityLevel, filter.AvailableAction}, "\x00")))
	var cursor datasetCursor
	if filter.Cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(filter.Cursor)
		if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Scope != scope {
			return out, controlError(400, "DATASET_CURSOR_INVALID")
		}
		if _, err := parseDatasetUUID(cursor.After); err != nil {
			return out, controlError(400, "DATASET_CURSOR_INVALID")
		}
	}
	entries, err := s.DatasetCatalog(ctx)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	observe := s.datasetProjector()
	var accounted []repo.DatasetRelationSample
	complete := true
	scanned := 0
	last := ""
	for _, entry := range entries {
		id := entry.DatasetUUID.String()
		if id <= cursor.After || filter.OwnerKind != "" && entry.OwnerKind != filter.OwnerKind ||
			filter.PluginID != "" && filter.PluginID != entry.PluginID ||
			filter.AvailableAction == "open_existing_policy" && entry.ExistingPolicy == "" {
			continue
		}
		if len(out.Items) == filter.Limit || scanned == 100 || last != "" && ctx.Err() != nil {
			raw, _ := json.Marshal(datasetCursor{Scope: scope, After: last})
			token := base64.RawURLEncoding.EncodeToString(raw)
			out.NextCursor = &token
			break
		}
		view, samples := observe(ctx, entry)
		last = id
		scanned++
		if filter.CapacityLevel != "" && view.Observation.Level != filter.CapacityLevel ||
			!datasetFilterValue(filter.AvailableAction, view.AvailableActions...) {
			continue
		}
		out.Items = append(out.Items, view)
		complete = complete && len(samples) > 0
		accounted = append(accounted, samples...)
	}
	if complete {
		if total, err := repo.SumDatasetPhysicalBytes(accounted); err == nil {
			out.PagePhysicalBytes = &total
		}
	}
	return out, nil
}

func (s *Service) DatasetDetails(ctx context.Context, rawID string) (DatasetView, error) {
	id, err := parseDatasetUUID(rawID)
	if err != nil {
		return DatasetView{}, controlError(400, "DATASET_ID_INVALID")
	}
	items, err := s.DatasetCatalog(ctx)
	if err != nil {
		return DatasetView{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, entry := range items {
		if entry.DatasetUUID == id {
			view, _ := s.datasetProjector()(ctx, entry)
			return view, nil
		}
	}
	return DatasetView{}, controlError(404, "DATASET_NOT_FOUND")
}

func datasetFilterValue(got string, allowed ...string) bool {
	if got == "" {
		return true
	}
	for _, v := range allowed {
		if got == v {
			return true
		}
	}
	return false
}

func (s *Service) datasetProjector() func(context.Context, DatasetCatalogEntry) (DatasetView, []repo.DatasetRelationSample) {
	var system map[string]repo.SystemTableObservation
	limits, _ := s.tableMonitoringPolicy(nil)
	return func(ctx context.Context, entry DatasetCatalogEntry) (DatasetView, []repo.DatasetRelationSample) {
		view := DatasetView{
			DatasetUUID: entry.DatasetUUID, DatasetKey: entry.DatasetKey, Name: entry.DatasetKey,
			OwnerKind: entry.OwnerKind, PluginID: entry.PluginID, TenantScope: "platform",
			SourceState: "unavailable", RegistrationState: entry.RegistrationState,
			AcceptanceRevision: entry.AcceptanceRevision, ContractVersion: entry.ContractVersion,
			GovernanceClass: "protected", AvailableActions: []string{"observe"},
			Observation: DatasetObservation{State: "unavailable", Level: "unknown", Coverage: "unknown"},
		}
		observed := func(total int64) {
			now := s.opts.Now().UTC()
			view.SourceState, view.Observation.State, view.Observation.Coverage = "ready", "observed", "complete"
			view.Observation.TotalBytes, view.Observation.ObservedAt = &total, &now
			view.Observation.Level = "normal"
			if total >= limits.WarningBytes {
				view.Observation.Level = "warning"
			}
		}
		if entry.OwnerKind == "system" {
			if entry.ExistingPolicy != "" {
				view.GovernanceClass = "retention_review"
				view.AvailableActions = append(view.AvailableActions, "open_existing_policy")
				view.ExistingPolicyRefs = []DatasetPolicyReference{{Kind: entry.ExistingPolicy, LegacyTableID: entry.DatasetKey}}
			}
			if system == nil {
				system = map[string]repo.SystemTableObservation{}
				if provider, ok := s.opts.DatabaseCollector.(SystemTableMetricsCollector); ok {
					if rows, err := provider.CollectSystemTableMetrics(ctx); err == nil {
						for _, row := range rows {
							system[row.Name] = row
						}
					}
				}
			}
			row := system[entry.DatasetKey]
			if row.State == "observed" && row.TotalBytes != nil {
				observed(*row.TotalBytes)
				// Built-in rows are unique and live in the same Core database.
				if s.opts.DatasetSources != nil {
					return view, builtinDatasetSample(s.opts.DatasetSources.ClusterID, row)
				}
			} else {
				view.StatusReason = "system_metrics_unavailable"
			}
			return view, nil
		}
		view.TenantScope, view.SourceBindingUUID = entry.Declaration.TenantScope, &entry.SourceBindingUUID
		view.Name, view.RetentionBounds = entry.Declaration.DisplayName["zh-CN"], entry.Declaration.RetentionBounds
		if view.RetentionBounds != nil {
			view.GovernanceClass = "retention_review"
		}
		if entry.RegistrationState == "retired" {
			view.StatusReason = "dataset_retired"
			return view, nil
		}
		source, err := s.resolveInstalledDataset(ctx, entry.PluginID, entry.DatasetKey)
		if err != nil {
			view.StatusReason = "dataset_binding_unavailable"
			return view, nil
		}
		if source.binding != entry.SourceBindingUUID || source.plugin.Revision != entry.Installation || source.definitionFingerprint() != entry.DefinitionDigest {
			view.SourceState, view.RegistrationState, view.StatusReason = "needs_revalidation", "needs_revalidation", "dataset_binding_changed"
			return view, nil
		}
		samples, fingerprint, err := s.sampleInstalledDataset(ctx, source)
		if err != nil {
			view.StatusReason = "dataset_source_metrics_unavailable"
			return view, nil
		}
		if entry.SourceFingerprint != fingerprint || entry.RegistrationState != "accepted" {
			view.SourceState, view.StatusReason = "needs_revalidation", "dataset_source_needs_revalidation"
			if entry.SourceFingerprint != "" {
				view.RegistrationState = "needs_revalidation"
			}
			return view, nil
		}
		total, err := repo.SumDatasetPhysicalBytes(samples)
		if err == nil {
			observed(total)
			if s.opts.DatasetTasks != nil && view.RetentionBounds != nil {
				view.AvailableActions = append(view.AvailableActions, "preview")
				if s.opts.DatasetExecutionEnabled && s.opts.Control != nil && s.opts.Control.executionFence != nil {
					if !entry.Declaration.ArchiveBeforeDelete && datasetFilterValue("delete", entry.Declaration.AllowedActions...) {
						view.AvailableActions = append(view.AvailableActions, "delete")
					}
					if s.recoveryArchives != nil && datasetFilterValue("archive", entry.Declaration.AllowedActions...) {
						view.AvailableActions = append(view.AvailableActions, "archive")
					}
				}
			}
			return view, samples
		}
		view.StatusReason = "dataset_source_metrics_unavailable"
		return view, nil
	}
}

func builtinDatasetSample(clusterID string, row repo.SystemTableObservation) []repo.DatasetRelationSample {
	_, oid, found := strings.Cut(row.DatabaseIdentity, ":")
	// Database names can contain colons. The last segment is the catalog OID.
	if pos := strings.LastIndexByte(row.DatabaseIdentity, ':'); pos >= 0 {
		oid = row.DatabaseIdentity[pos+1:]
	}
	dbID, err := strconv.ParseInt(oid, 10, 64)
	if !found || err != nil || clusterID == "" || dbID <= 0 || row.RelationID <= 0 {
		return nil
	}
	return []repo.DatasetRelationSample{{
		Identity: repo.DatasetRelationIdentity{ClusterID: clusterID, DatabaseID: dbID, RelationID: row.RelationID},
		Bytes:    *row.TotalBytes,
	}}
}
