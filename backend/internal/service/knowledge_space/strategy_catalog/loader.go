package strategy_catalog

import (
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type rawCatalog struct {
	Version int                  `yaml:"version"`
	Kind    string               `yaml:"kind"`
	StrategyPackages map[string]rawStrategyPackage `yaml:"strategy_packages"`
	Bundles          map[string]rawBundle          `yaml:"strategy_bundles"`
	Scenes           map[string]rawScene           `yaml:"scenes"`
}

type rawStrategyPackage struct {
	Label                 string               `yaml:"label"`
	Summary               string               `yaml:"summary"`
	Coupling              string               `yaml:"coupling"`
	RecommendedProfileKey string               `yaml:"recommended_profile_key"`
	RecommendedScenes     []string             `yaml:"recommended_scenes"`
	Dependencies          rawStrategyDepends    `yaml:"dependencies"`
}

type rawStrategyDepends struct {
	Index   []string `yaml:"index"`
	Runtime []string `yaml:"runtime"`
	Assets  []string `yaml:"assets"`
}

type rawBundle struct {
	Label         string   `yaml:"label"`
	Description   string   `yaml:"description"`
	Prerequisites []string `yaml:"prerequisites"`
}

type rawScene struct {
	Label          string          `yaml:"label"`
	Description    string          `yaml:"description"`
	DefaultBundle  string          `yaml:"default_bundle"`
	AllowedBundles []string        `yaml:"allowed_bundles"`
	Prerequisites  rawScenePrereq  `yaml:"prerequisites"`
	Ingestion      rawIngestionDef `yaml:"ingestion_defaults"`
}

type rawScenePrereq struct {
	Index  []string `yaml:"index"`
	Assets []string `yaml:"assets"`
}

type rawIngestionDef struct {
	Chunking rawChunkingDef `yaml:"chunking"`
}

type rawChunkingDef struct {
	Mode       string   `yaml:"mode"`
	Unit       string   `yaml:"unit"`
	ChunkSize  int      `yaml:"chunk_size"`
	Overlap    int      `yaml:"overlap"`
	Separators []string `yaml:"separators"`
}

type Loader struct {
	path string
	mu   sync.Mutex

	cached       *Catalog
	cachedMTime  time.Time
	cachedLoaded bool
}

func NewLoader(path string) *Loader {
	return &Loader{path: path}
}

func (l *Loader) Load() (*Catalog, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	st, err := os.Stat(l.path)
	if err != nil {
		return nil, err
	}

	if l.cachedLoaded && l.cached != nil && !st.ModTime().After(l.cachedMTime) {
		return l.cached, nil
	}

	raw, err := os.ReadFile(l.path)
	if err != nil {
		return nil, err
	}

	var rc rawCatalog
	if err := yaml.Unmarshal(raw, &rc); err != nil {
		return nil, err
	}

	out := &Catalog{
		Version: rc.Version,
		Kind:    rc.Kind,
		StrategyPackages: map[string]StrategyPackage{},
		Bundles:          map[string]StrategyBundle{},
		Scenes:           map[string]Scene{},
	}

	for k, p := range rc.StrategyPackages {
		out.StrategyPackages[k] = StrategyPackage{
			Key:                   k,
			Label:                 p.Label,
			Summary:               p.Summary,
			Coupling:              p.Coupling,
			RecommendedProfileKey: p.RecommendedProfileKey,
			RecommendedScenes:     append([]string(nil), p.RecommendedScenes...),
			Dependencies: StrategyDependencies{
				Index:   append([]string(nil), p.Dependencies.Index...),
				Runtime: append([]string(nil), p.Dependencies.Runtime...),
				Assets:  append([]string(nil), p.Dependencies.Assets...),
			},
		}
	}

	for k, b := range rc.Bundles {
		out.Bundles[k] = StrategyBundle{
			Key:           k,
			Label:         b.Label,
			Description:   b.Description,
			Prerequisites: append([]string(nil), b.Prerequisites...),
		}
	}
	for k, sc := range rc.Scenes {
		out.Scenes[k] = Scene{
			Key:            k,
			Label:          sc.Label,
			Description:    sc.Description,
			DefaultBundle:  sc.DefaultBundle,
			AllowedBundles: append([]string(nil), sc.AllowedBundles...),
			Prerequisites: ScenePrerequisites{
				Index:  append([]string(nil), sc.Prerequisites.Index...),
				Assets: append([]string(nil), sc.Prerequisites.Assets...),
			},
			Ingestion: SceneIngestionDefaults{
				Chunking: SceneChunkingDefaults{
					Mode:       sc.Ingestion.Chunking.Mode,
					Unit:       sc.Ingestion.Chunking.Unit,
					ChunkSize:  sc.Ingestion.Chunking.ChunkSize,
					Overlap:    sc.Ingestion.Chunking.Overlap,
					Separators: append([]string(nil), sc.Ingestion.Chunking.Separators...),
				},
			},
		}
	}

	l.cached = out
	l.cachedMTime = st.ModTime()
	l.cachedLoaded = true
	return out, nil
}
