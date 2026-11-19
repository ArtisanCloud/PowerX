package storagegov

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var datasetKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
var datasetDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// DatasetDeclaration is the admitted declaration from a verified installed
// artifact. It contains logical keys, never database identifiers or connection data.
type DatasetDeclaration struct {
	DatasetKey            string                  `json:"dataset_key"`
	DisplayName           map[string]string       `json:"display_name_i18n"`
	ContractVersion       int64                   `json:"contract_version"`
	SchemaContractVersion int64                   `json:"schema_contract_version"`
	SourceBindingRef      string                  `json:"source_binding_ref"`
	TenantScope           string                  `json:"tenant_scope"`
	AdapterKey            string                  `json:"adapter_key"`
	AdapterVersion        int64                   `json:"adapter_version"`
	AllowedActions        []string                `json:"allowed_actions"`
	RetentionBounds       *DatasetRetentionBounds `json:"retention_bounds,omitempty"`
	ArchiveBeforeDelete   bool                    `json:"archive_before_delete,omitempty"`
}

func (d DatasetDeclaration) validate() bool {
	if !datasetKeyPattern.MatchString(d.DatasetKey) || !datasetKeyPattern.MatchString(d.SourceBindingRef) ||
		!datasetKeyPattern.MatchString(d.AdapterKey) || d.TenantScope != "tenant" ||
		!datasetRevisionOK(d.ContractVersion) || !datasetRevisionOK(d.SchemaContractVersion) || !datasetRevisionOK(d.AdapterVersion) ||
		len(d.AllowedActions) < 1 || len(d.AllowedActions) > 4 || d.DisplayName["zh-CN"] == "" || d.DisplayName["en"] == "" {
		return false
	}
	for _, label := range d.DisplayName {
		if label == "" || len([]rune(label)) > 128 {
			return false
		}
	}
	seen := map[string]bool{}
	for _, action := range d.AllowedActions {
		if !slices.Contains([]string{"observe", "preview", "delete", "archive"}, action) || seen[action] {
			return false
		}
		seen[action] = true
	}
	if !seen["observe"] || seen["preview"] != (d.RetentionBounds != nil) ||
		(seen["delete"] || seen["archive"]) && !seen["preview"] ||
		d.ArchiveBeforeDelete && (!seen["delete"] || !seen["archive"]) {
		return false
	}
	return !seen["preview"] || d.RetentionBounds.Validate() == nil
}

func datasetRevisionOK(revision int64) bool { return revision > 0 && revision <= 1<<53-1 }

// VerifiedDatasetAdmission is assembled by a trusted host resolver after
// checking the installed artifact, active tenant and database binding.
// It must never be decoded directly from a public request.
type VerifiedDatasetAdmission struct {
	RequestUUID       string
	Actor             string
	CallerPluginID    string
	TenantUUID        uuid.UUID
	OwnerID           string
	Installation      string
	BindingUUID       uuid.UUID
	DefinitionDigest  string
	SourceFingerprint string
	Declaration       DatasetDeclaration
}

// DatasetAdmissionReceipt is the original outcome of one registration request.
type DatasetAdmissionReceipt struct {
	RequestUUID        uuid.UUID `json:"request_uuid"`
	DatasetUUID        uuid.UUID `json:"dataset_uuid"`
	AcceptanceRevision int64     `json:"acceptance_revision"`
	RegistrationState  string    `json:"registration_state"`
}

// AdmitVerifiedDataset persists a verified plugin declaration without touching
// the plugin source database.
func (s *Service) AdmitVerifiedDataset(ctx context.Context, in VerifiedDatasetAdmission) (DatasetAdmissionReceipt, error) {
	if s.opts.DB == nil || s.opts.DB.Dialector.Name() != "postgres" || !idPattern.MatchString(s.opts.Identity.InstanceID) {
		return DatasetAdmissionReceipt{}, ErrStateUnavailable
	}
	if in.CallerPluginID != in.OwnerID || in.TenantUUID == uuid.Nil || in.Actor == "" {
		return DatasetAdmissionReceipt{}, controlError(403, "DATASET_OWNER_REQUIRED")
	}
	requestID, err := parseDatasetUUID(in.RequestUUID)
	if err != nil || !idPattern.MatchString(in.OwnerID) || !idPattern.MatchString(in.Installation) || in.BindingUUID == uuid.Nil ||
		len(in.Actor) > 256 || !datasetDigestPattern.MatchString(in.DefinitionDigest) ||
		(in.SourceFingerprint != "" && !datasetDigestPattern.MatchString(in.SourceFingerprint)) || !in.Declaration.validate() {
		return DatasetAdmissionReceipt{}, controlError(400, "DATASET_DECLARATION_INVALID")
	}
	raw, err := json.Marshal(in.Declaration)
	if err != nil {
		return DatasetAdmissionReceipt{}, err
	}
	// Source verification changes independently of the submitted request.
	// Bind identity, installation and declared content to the immutable receipt.
	identity, err := json.Marshal([]string{in.Actor, in.TenantUUID.String(), in.OwnerID, in.Installation, in.BindingUUID.String(), in.DefinitionDigest, string(raw)})
	if err != nil {
		return DatasetAdmissionReceipt{}, err
	}
	digest := sha256.Sum256(identity)
	row := m.DatasetRegistration{
		OwnerID: in.OwnerID, BindingUUID: in.BindingUUID, DatasetKey: in.Declaration.DatasetKey,
		RequestUUID: &requestID, RequestDigest: hex.EncodeToString(digest[:]), Actor: in.Actor, ServiceTenantUUID: in.TenantUUID,
		Installation:     in.Installation,
		DefinitionDigest: in.DefinitionDigest, SourceFingerprint: in.SourceFingerprint, DefinitionVersion: in.Declaration.ContractVersion,
		Declaration: raw, State: "accepted",
	}
	if in.SourceFingerprint == "" {
		row.State = "pending_validation"
	}
	r := repo.NewDatasetRepository(s.opts.DB, s.opts.Identity.InstanceID)
	err = r.WithAdmission(ctx, func(tx *repo.DatasetRepository) error {
		prior, e := tx.Request(ctx, requestID)
		if e == nil {
			if prior.RequestDigest != row.RequestDigest {
				return controlError(409, "DATASET_REQUEST_CONFLICT")
			}
			row = prior
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		head, e := tx.Head(ctx, row.OwnerID, row.BindingUUID, row.DatasetKey)
		if errors.Is(e, gorm.ErrRecordNotFound) {
			row.DatasetUUID, row.AcceptanceRevision, row.IsCurrent = uuid.New(), 1, true
			return tx.Append(ctx, nil, &row)
		}
		if e != nil {
			return e
		}
		if head.DeletedAt.Valid || head.Status != 1 || head.State == "retired" || head.DefinitionVersion > row.DefinitionVersion ||
			(head.DefinitionVersion == row.DefinitionVersion && head.DefinitionDigest != row.DefinitionDigest) {
			return controlError(409, "DATASET_DEFINITION_CONFLICT")
		}
		row.DatasetUUID, row.AcceptanceRevision = head.DatasetUUID, head.AcceptanceRevision
		if head.DefinitionVersion == row.DefinitionVersion && head.SourceFingerprint == row.SourceFingerprint &&
			head.State == row.State && head.Installation == row.Installation {
			return tx.Append(ctx, nil, &row)
		}
		row.AcceptanceRevision++
		if !datasetRevisionOK(row.AcceptanceRevision) {
			return ErrStateUnavailable
		}
		if head.SourceFingerprint != "" && head.SourceFingerprint != row.SourceFingerprint {
			row.State = "needs_revalidation"
		}
		row.IsCurrent = true
		return tx.Append(ctx, &head, &row)
	})
	if err != nil {
		return DatasetAdmissionReceipt{}, err
	}
	return DatasetAdmissionReceipt{requestID, row.DatasetUUID, row.AcceptanceRevision, row.State}, nil
}
