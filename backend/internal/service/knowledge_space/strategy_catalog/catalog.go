package strategy_catalog

import "time"

type Catalog struct {
	Version int
	Kind    string
	StrategyPackages map[string]StrategyPackage
	Bundles          map[string]StrategyBundle
	Scenes           map[string]Scene
}

type StrategyPackage struct {
	Key                   string
	Label                 string
	Summary               string
	Coupling              string
	RecommendedProfileKey string
	RecommendedScenes     []string
	Dependencies          StrategyDependencies
}

type StrategyDependencies struct {
	Index   []string
	Runtime []string
	Assets  []string
}

type StrategyBundle struct {
	Key           string
	Label         string
	Description   string
	Prerequisites []string
}

type Scene struct {
	Key            string
	Label          string
	Description    string
	DefaultBundle  string
	AllowedBundles []string
	Prerequisites  ScenePrerequisites
	Ingestion      SceneIngestionDefaults
}

type ScenePrerequisites struct {
	Index  []string
	Assets []string
}

type SceneIngestionDefaults struct {
	Chunking SceneChunkingDefaults
}

type SceneChunkingDefaults struct {
	Mode       string
	Unit       string
	ChunkSize  int
	Overlap    int
	Separators []string
}

type ValidationResult struct {
	OK              bool            `json:"ok"`
	SceneKey        string          `json:"sceneKey"`
	BundleKey       string          `json:"bundleKey"`
	EnabledChannels []string        `json:"enabledChannels"`
	Missing         []MissingPrereq `json:"missing"`
	Capabilities    map[string]bool `json:"capabilities"`
	CheckedAt       time.Time       `json:"checkedAt"`
}

type MissingPrereq struct {
	Code        string   `json:"code"`
	Key         string   `json:"key"`
	Message     string   `json:"message"`
	Remediation []string `json:"remediation"`
}
