package metadata

type TagResponse struct {
	UUID            string  `json:"uuid"`
	Namespace       string  `json:"namespace"`
	ResourceType    string  `json:"resource_type"`
	Code            string  `json:"code"`
	LabelI18n       I18nMap `json:"label_i18n"`
	DescriptionI18n I18nMap `json:"description_i18n,omitempty"`
	Color           string  `json:"color,omitempty"`
	Status          string  `json:"status"`
	UsageCount      int64   `json:"usage_count"`
	Display
}
