package storagegov

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/google/uuid"
)

const maxDatasetCatalogHeads = 1000

var systemDatasetNamespace = uuid.MustParse("b8fe8501-957f-439f-a25a-a372d1cdd290")

// DatasetCatalogEntry projects both admission sources without accessing data
// payloads. ExistingPolicy identifies the unchanged system controls.
type DatasetCatalogEntry struct {
	DatasetUUID        uuid.UUID
	DatasetKey         string
	OwnerKind          string
	PluginID           string
	SourceBindingUUID  uuid.UUID
	AcceptanceRevision int64
	ContractVersion    int64
	RegistrationState  string
	Installation       string
	DefinitionDigest   string
	SourceFingerprint  string
	ExistingPolicy     string
	Declaration        *DatasetDeclaration
}

// DatasetCatalog lists built-in and admitted datasets for the platform
// operator within this deployment.
func (s *Service) DatasetCatalog(ctx context.Context) ([]DatasetCatalogEntry, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return nil, ErrRootRequired
	}
	if s.opts.DB == nil || !idPattern.MatchString(s.opts.Identity.InstanceID) {
		return nil, ErrStateUnavailable
	}
	heads, err := repo.NewDatasetRepository(s.opts.DB, s.opts.Identity.InstanceID).Heads(ctx, maxDatasetCatalogHeads+1)
	if err != nil || len(heads) > maxDatasetCatalogHeads {
		return nil, ErrStateUnavailable
	}
	items := make([]DatasetCatalogEntry, 0, len(heads)+len(m.SystemTableCatalog()))
	for _, builtin := range m.SystemTableCatalog() {
		entry := DatasetCatalogEntry{
			DatasetUUID: uuid.NewSHA1(systemDatasetNamespace, []byte(s.opts.Identity.InstanceID+"\x00"+builtin.Name)),
			DatasetKey:  builtin.Name, OwnerKind: "system", RegistrationState: "builtin",
		}
		switch {
		case repo.SystemRetentionSupported(builtin.Name):
			entry.ExistingPolicy = "system_table_retention"
		case builtin.CleanupMechanism == "invocation_policy":
			entry.ExistingPolicy = "invocation_retention"
		}
		items = append(items, entry)
	}
	for _, head := range heads {
		var declaration DatasetDeclaration
		if err := json.Unmarshal(head.Declaration, &declaration); err != nil || !declaration.validate() ||
			declaration.DatasetKey != head.DatasetKey || declaration.ContractVersion != head.DefinitionVersion {
			return nil, ErrStateUnavailable
		}
		items = append(items, DatasetCatalogEntry{
			DatasetUUID: head.DatasetUUID, DatasetKey: head.DatasetKey, OwnerKind: "plugin", PluginID: head.OwnerID,
			SourceBindingUUID: head.BindingUUID, AcceptanceRevision: head.AcceptanceRevision, ContractVersion: head.DefinitionVersion,
			RegistrationState: head.State, Declaration: &declaration, Installation: head.Installation,
			DefinitionDigest: head.DefinitionDigest, SourceFingerprint: head.SourceFingerprint,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].DatasetUUID.String() < items[j].DatasetUUID.String()
	})
	return items, nil
}
