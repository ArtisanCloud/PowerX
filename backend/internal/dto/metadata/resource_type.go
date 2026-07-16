package metadata

type ResourceTypeResponse struct {
	UUID            string  `json:"uuid"`
	ResourceType    string  `json:"resource_type"`
	Module          string  `json:"module"`
	NameI18n        I18nMap `json:"name_i18n"`
	DescriptionI18n I18nMap `json:"description_i18n,omitempty"`
	ValidatorKey    string  `json:"validator_key,omitempty"`
	BindingEnabled  bool    `json:"binding_enabled"`
	ValidatorStatus string  `json:"validator_status"`
	Status          string  `json:"status"`
	Display
}
