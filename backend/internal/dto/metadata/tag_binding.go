package metadata

type TagBindingResponse struct {
	BindingUUID  string       `json:"binding_uuid"`
	TagUUID      string       `json:"tag_uuid"`
	ResourceType string       `json:"resource_type"`
	ResourceUUID string       `json:"resource_uuid"`
	Tag          *TagResponse `json:"tag,omitempty"`
}
