package metadata

type DictionaryNamespaceResponse struct {
	UUID            string  `json:"uuid"`
	Namespace       string  `json:"namespace"`
	Module          string  `json:"module"`
	NameI18n        I18nMap `json:"name_i18n"`
	DescriptionI18n I18nMap `json:"description_i18n,omitempty"`
	Status          string  `json:"status"`
	ItemCount       int64   `json:"item_count"`
	Display
}

type DictionaryItemResponse struct {
	UUID            string  `json:"uuid"`
	NamespaceUUID   string  `json:"namespace_uuid"`
	Code            string  `json:"code"`
	LabelI18n       I18nMap `json:"label_i18n"`
	DescriptionI18n I18nMap `json:"description_i18n,omitempty"`
	Status          string  `json:"status"`
	SortOrder       int     `json:"sort_order"`
	ReferenceCount  int64   `json:"reference_count"`
	Display
}
