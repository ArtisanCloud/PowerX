package capability_registrydto

// GrantStatusRequest contains only the capabilities for which the current
// service credential requests its own effective grant status.
type GrantStatusRequest struct {
	CapabilityIDs []string `json:"capability_ids"`
}

type GrantStatusItem struct {
	CapabilityID string `json:"capability_id"`
	Status       string `json:"status"`
	ReasonCode   string `json:"reason_code"`
}

type GrantStatusResponse struct {
	Items []GrantStatusItem `json:"items"`
}
