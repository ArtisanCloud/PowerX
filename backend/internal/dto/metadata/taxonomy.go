package metadata

type TaxonomyResponse struct {
	UUID            string  `json:"uuid"`
	Namespace       string  `json:"namespace"`
	Module          string  `json:"module"`
	NameI18n        I18nMap `json:"name_i18n"`
	DescriptionI18n I18nMap `json:"description_i18n,omitempty"`
	MaxDepth        int     `json:"max_depth"`
	Status          string  `json:"status"`
	Display
}

type TaxonomyNodeResponse struct {
	UUID            string  `json:"uuid"`
	TaxonomyUUID    string  `json:"taxonomy_uuid"`
	ParentUUID      *string `json:"parent_uuid,omitempty"`
	Code            string  `json:"code"`
	LabelI18n       I18nMap `json:"label_i18n"`
	DescriptionI18n I18nMap `json:"description_i18n,omitempty"`
	Path            string  `json:"path"`
	Depth           int     `json:"depth"`
	SortOrder       int     `json:"sort_order"`
	Status          string  `json:"status"`
	ReferenceCount  int64   `json:"reference_count"`
	Version         int64   `json:"version"`
	Display
}
