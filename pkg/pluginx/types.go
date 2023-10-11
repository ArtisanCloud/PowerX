package pluginx

type SystemPlatform string

const (
	WINDOWS SystemPlatform = "windows"
	LINUX   SystemPlatform = "linux"
	MACOS   SystemPlatform = "macos"
)

type BuildPluginItem struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
	Backend     string `yaml:"backend"`
}

type BuildInfo struct {
	BuildDate string            `yaml:"buildDate"`
	Plugins   []BuildPluginItem `yaml:"plugins"`
}

type PluginEtc struct {
	Name   string       `yaml:"name"`
	Enable bool         `yaml:"enable"`
	Etc    PluginEtcMap `yaml:"etc"`
}
type PluginEtcMap = map[string]any

type PluginEtcItemPluginWrapper struct {
	Plugin PluginEtcMap `yaml:"plugin"`
}

type PluginManagerEtc struct {
	Plugins []PluginEtc `yaml:"plugins"`
}

type PluginFrontendRoute struct {
	Name string `yaml:"name" json:"name"`
	Path string `yaml:"path" json:"path"`
	Meta struct {
		Icon   string `yaml:"icon" json:"icon"`
		Locale string `yaml:"locale" json:"locale"`
	} `yaml:"meta" json:"meta"`
}

type PluginFrontendInfo struct {
	Routes []PluginFrontendRoute `yaml:"routes" json:"routes"`
}

type BuildData struct {
	Build    BuildInfo          `yaml:"build"`
	Etc      PluginManagerEtc   `yaml:"etc"`
	Frontend PluginFrontendInfo `yaml:"frontend"`
}
