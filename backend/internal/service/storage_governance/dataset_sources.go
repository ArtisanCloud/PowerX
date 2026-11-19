package storagegov

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"io"
	"os"
	"regexp"
	"sort"
	"syscall"

	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
)

const datasetAssetMaxBytes = 16384

var datasetRelationName = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// DatasetSources resolves the trusted installation and gives the observer its
// allowed schema and expected role. These callbacks are provided by Core.
type DatasetSources struct {
	ClusterID string
	Installed func(context.Context, string) (pm.Plugin, error)
	Binding   func(pm.Plugin) (uuid.UUID, string, string, error)
	External  map[string]DatasetExternalSource
}

// DatasetExternalSource is bound by Core deployment config. Observe opens
// only the fixed read-only catalog connection and releases it after sampling.
type DatasetExternalSource struct {
	Binding   uuid.UUID
	ClusterID string
	Schema    string
	Role      string
	Open      func() (*gorm.DB, func(), error)
}

type installedDataset struct {
	plugin      pm.Plugin
	external    *DatasetExternalSource
	binding     uuid.UUID
	schema      string
	role        string
	spec        pm.DatasetObservationSpec
	declaration DatasetDeclaration
}

func (s *Service) resolveInstalledDataset(ctx context.Context, pluginID, key string) (installedDataset, error) {
	var out installedDataset
	sources := s.opts.DatasetSources
	if sources == nil || sources.Installed == nil {
		return out, controlError(503, "DATASET_INSTALLATION_UNAVAILABLE")
	}
	plugin, err := sources.Installed(ctx, pluginID)
	if err != nil || plugin.ID != pluginID || plugin.State != pm.StateEnabled || !idPattern.MatchString(plugin.Revision) {
		return out, controlError(403, "DATASET_INSTALLATION_FORBIDDEN")
	}
	spec, ok := plugin.DataLifecycle[key]
	if !ok || !datasetKeyPattern.MatchString(key) || !datasetDigestPattern.MatchString(spec.SHA256) ||
		len(spec.Tables) < 1 || len(spec.Tables) > 32 {
		return out, controlError(422, "DATASET_DECLARATION_UNREGISTERED")
	}
	seen := map[string]bool{}
	for _, table := range spec.Tables {
		if !datasetRelationName.MatchString(table) || seen[table] {
			return out, controlError(422, "DATASET_OBSERVATION_MEMBERS_INVALID")
		}
		seen[table] = true
	}
	declaration, err := readInstalledDataset(plugin.Paths.Root, key, spec.SHA256)
	if err != nil {
		return out, controlError(409, "DATASET_DEFINITION_CONFLICT")
	}
	result := installedDataset{plugin: plugin, spec: spec, declaration: declaration}
	if external, ok := sources.External[plugin.ID]; ok {
		if external.Binding == uuid.Nil || external.Open == nil || !idPattern.MatchString(external.ClusterID) ||
			!datasetRelationName.MatchString(external.Schema) || !datasetRelationName.MatchString(external.Role) {
			return out, controlError(422, "DATASET_SOURCE_BINDING_UNRESOLVED")
		}
		result.binding, result.schema, result.role, result.external = external.Binding, external.Schema, external.Role, &external
		return result, nil
	}
	if sources.Binding == nil {
		return out, controlError(422, "DATASET_SOURCE_BINDING_UNRESOLVED")
	}
	binding, schema, role, err := sources.Binding(plugin)
	if err != nil || binding == uuid.Nil || !datasetRelationName.MatchString(schema) || !datasetRelationName.MatchString(role) {
		return out, controlError(422, "DATASET_SOURCE_BINDING_UNRESOLVED")
	}
	result.binding, result.schema, result.role = binding, schema, role
	return result, nil
}

// definitionFingerprint binds the installed observation allowlist and the
// separately pinned declaration asset to the same definition revision.
func (d installedDataset) definitionFingerprint() string {
	members := append([]string(nil), d.spec.Tables...)
	sort.Strings(members)
	raw, _ := json.Marshal(append([]string{d.spec.SHA256}, members...))
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func readInstalledDataset(root, key, expectedDigest string) (DatasetDeclaration, error) {
	var declaration DatasetDeclaration
	installRoot, err := os.OpenRoot(root)
	if err != nil {
		return declaration, err
	}
	defer installRoot.Close()
	file, err := installRoot.OpenFile("contracts/storage-datasets/"+key+".json", os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return declaration, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > datasetAssetMaxBytes {
		return declaration, errors.New("dataset declaration file invalid")
	}
	raw, err := io.ReadAll(io.LimitReader(file, datasetAssetMaxBytes+1))
	if err != nil || len(raw) > datasetAssetMaxBytes {
		return declaration, errors.New("dataset declaration unreadable")
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != expectedDigest {
		return declaration, errors.New("dataset declaration digest mismatch")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&declaration) != nil || decoder.Decode(&struct{}{}) != io.EOF || !declaration.validate() || declaration.DatasetKey != key {
		return DatasetDeclaration{}, errors.New("dataset declaration invalid")
	}
	return declaration, nil
}

func (s *Service) sampleInstalledDataset(ctx context.Context, source installedDataset) ([]repo.DatasetRelationSample, string, error) {
	return s.sampleInstalledDatasetTx(ctx, source, nil)
}

func (s *Service) sampleInstalledDatasetTx(ctx context.Context, source installedDataset, coreTx *gorm.DB) ([]repo.DatasetRelationSample, string, error) {
	if s.opts.DatasetSources == nil {
		return nil, "", ErrStateUnavailable
	}
	db, cluster := s.opts.DB, s.opts.DatasetSources.ClusterID
	if source.external != nil {
		conn, closeConn, err := source.external.Open()
		if err != nil {
			return nil, "", ErrStateUnavailable
		}
		defer closeConn()
		db, cluster = conn, source.external.ClusterID
	}
	if !idPattern.MatchString(cluster) {
		return nil, "", ErrStateUnavailable
	}
	var samples []repo.DatasetRelationSample
	var err error
	if source.external == nil && coreTx != nil {
		samples, err = repo.ObserveBoundDatasetRelationsTx(ctx, coreTx, cluster, source.schema, source.role, source.spec.Tables)
	} else {
		samples, err = repo.ObserveBoundDatasetRelations(ctx, db, cluster, source.schema, source.role, source.spec.Tables)
	}
	if err != nil {
		return nil, "", err
	}
	// Fingerprint stable relation identities, not their changing byte counts.
	members := make([]string, 0, len(samples))
	for _, entry := range samples {
		id := entry.Identity
		members = append(members, fmt.Sprintf("%s:%d:%d", id.ClusterID, id.DatabaseID, id.RelationID))
	}
	sort.Strings(members)
	raw, _ := json.Marshal(members)
	digest := sha256.Sum256(raw)
	return samples, hex.EncodeToString(digest[:]), nil
}
