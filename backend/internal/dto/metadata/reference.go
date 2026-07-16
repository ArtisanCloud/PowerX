package metadata

type MetadataReferenceResponse struct {
	MetadataType string `json:"metadata_type"`
	MetadataUUID string `json:"metadata_uuid"`
	ResourceType string `json:"resource_type"`
	ResourceUUID string `json:"resource_uuid"`
	FieldName    string `json:"field_name"`
}
