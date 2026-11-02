package storagegov

// ResourceRef 仅包含部署登记的身份；测算结果不接受调用者覆盖环境、实例或路径。
type ResourceRef struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Scope      string `json:"scope"`
	InstanceID string `json:"instance_id,omitempty"`
}

type PolicyPreset struct {
	Version                int    `json:"version"`
	Intent                 string `json:"intent"`
	RetentionMode          string `json:"retention_mode"`
	TargetDays             int    `json:"target_days"`
	MaxBytes               *int64 `json:"max_bytes"`
	RetentionCount         int    `json:"retention_count"`
	MinVerifiedPoints      int    `json:"min_verified_points"`
	ProtectLatest          bool   `json:"protect_latest"`
	RequireVerifiedArchive bool   `json:"require_verified_archive"`
	SuggestedCadence       string `json:"suggested_cadence"`
}

type ResourcePolicyOption struct {
	Resource        ResourceRef  `json:"resource"`
	AllowedModes    []string     `json:"allowed_modes"`
	Preset          PolicyPreset `json:"preset"`
	ProtectionRules []string     `json:"protection_rules"`
	Cleanup         *LogControl  `json:"cleanup,omitempty"`
}

func refFor(r Resource) ResourceRef {
	return ResourceRef{ID: r.ID, Name: r.Name, Kind: r.Kind, Scope: r.Scope, InstanceID: r.InstanceID}
}

// policyOption 给出可编辑的建议，不生成运行策略、不修改旧调度、不代表已配置远端。
func policyOption(r Resource) ResourcePolicyOption {
	const gib int64 = 1 << 30
	bytes := func(n int64) *int64 { return &n }
	production := r.Scope == "prod" || r.Scope == "staging"
	p := PolicyPreset{Version: 1, Intent: r.Scope, RetentionMode: "days", TargetDays: 3, ProtectLatest: true, RequireVerifiedArchive: production, SuggestedCadence: "native_rotation"}
	o := ResourcePolicyOption{Resource: refFor(r), AllowedModes: []string{"days", "protected"}, ProtectionRules: []string{"active_work", "references", "explicit_hold"}}
	switch r.Kind {
	case "backup_files":
		p.RetentionMode, p.TargetDays, p.RetentionCount, p.MaxBytes, p.SuggestedCadence = "count", 0, 24, bytes(gib), "hourly"
		if production {
			p.RetentionCount, p.MaxBytes, p.SuggestedCadence = 2, bytes(6*gib), "daily"
		}
		o.AllowedModes = []string{"count"}
		o.ProtectionRules = append(o.ProtectionRules, "latest_success")
	case "snapshot_files":
		p.RetentionMode, p.TargetDays, p.RetentionCount, p.MinVerifiedPoints, p.MaxBytes, p.SuggestedCadence = "count", 0, 1, 1, bytes(gib), "on_demand"
		if production {
			p.RetentionCount, p.MinVerifiedPoints, p.MaxBytes = 2, 2, bytes(6*gib)
		}
		o.AllowedModes = []string{"count"}
		o.ProtectionRules = append(o.ProtectionRules, "verified_restore_points")
	case "postgresql_logs":
		p.TargetDays, p.MaxBytes = 7, bytes(3*gib)
		p.RequireVerifiedArchive = true
		o.ProtectionRules = append(o.ProtectionRules, "sealed_only")
	case "journal":
		p.TargetDays, p.MaxBytes = 2, bytes(3*gib)
		p.RequireVerifiedArchive = true
		o.ProtectionRules = append(o.ProtectionRules, "native_retention")
	case "loki":
		p.TargetDays, p.MaxBytes = 1, bytes(gib)
		o.ProtectionRules = append(o.ProtectionRules, "native_retention")
	case "temporary_files":
		p.TargetDays, p.MaxBytes, p.SuggestedCadence, p.RequireVerifiedArchive = 1, bytes(gib), "lifecycle", false
		if production {
			p.MaxBytes = bytes(2 * gib)
		}
		o.ProtectionRules = append(o.ProtectionRules, "lease_expired")
	case "application_logs":
		p.TargetDays, p.MaxBytes = 3, bytes(gib/4)
		if r.Scope == "test" {
			p.TargetDays = 1
		}
		if production {
			p.TargetDays, p.MaxBytes = 7, bytes(gib)
		}
		o.ProtectionRules = append(o.ProtectionRules, "sealed_only")
	case "media_files", "archive_objects":
		p.RetentionMode, p.TargetDays, p.RequireVerifiedArchive, p.SuggestedCadence = "protected", 0, true, "lifecycle"
		o.AllowedModes = []string{"protected"}
		o.ProtectionRules = append(o.ProtectionRules, "reference_graph")
	}
	if r.Scope == "shared" {
		p.Intent = "shared"
	}
	o.Preset = p
	return o
}

func (s *Service) policyOptions() []ResourcePolicyOption {
	out := make([]ResourcePolicyOption, 0, len(s.opts.Resources))
	for _, r := range s.opts.Resources {
		o := policyOption(r)
		if s.opts.LogLifecycle != nil {
			o.Cleanup = s.opts.LogLifecycle.Control(r.ID)
		}
		out = append(out, o)
	}
	return out
}

func validResourcePolicy(p ResourcePolicy, r Resource) bool {
	if p.TargetDays < 0 || p.TargetDays > 3650 || p.RetentionCount < 0 || p.RetentionCount > 10000 || p.MinVerifiedPoints < 0 || p.MinVerifiedPoints > p.RetentionCount {
		return false
	}
	// 旧版本草稿的组合字段原样保留；只有用户显式选新模式才改变语义。
	if p.RetentionMode == "" {
		return p.TargetDays >= 1
	}
	allowed := false
	for _, mode := range policyOption(r).AllowedModes {
		if p.RetentionMode == mode {
			allowed = true
		}
	}
	if !allowed {
		return false
	}
	switch p.RetentionMode {
	case "days":
		return p.TargetDays >= 1 && p.RetentionCount == 0 && p.MinVerifiedPoints == 0
	case "count":
		return p.TargetDays == 0 && p.RetentionCount >= 1
	case "protected":
		return p.TargetDays == 0 && p.RetentionCount == 0 && p.MinVerifiedPoints == 0
	}
	return false
}
