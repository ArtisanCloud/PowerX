package ops

// DeployReleaseRequest 为发布动作请求 DTO 骨架。
type DeployReleaseRequest struct {
	Environment    string `json:"environment"`
	BackendVersion string `json:"backend_version"`
	WebAdminVer    string `json:"web_admin_version"`
}

// DeployRollbackRequest 为回滚动作请求 DTO 骨架。
type DeployRollbackRequest struct {
	Environment   string `json:"environment"`
	TargetVersion string `json:"target_version"`
}
