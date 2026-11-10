package storagegov

import "time"

type CapacityAlert struct {
	Code       string `json:"code"`
	Level      string `json:"level"`
	ResourceID string `json:"resource_id,omitempty"`
}

// EvaluateCapacity 是当前观测的风险提示；不冒充已送达的告警或恢复通知。
func EvaluateCapacity(in Overview) []CapacityAlert {
	out := []CapacityAlert{}
	fs := in.Filesystem
	if fs.State != "observed" || fs.AvailableBytes == nil || fs.TotalBytes == nil || fs.UsedBytes == nil || *fs.TotalBytes <= 0 {
		out = append(out, CapacityAlert{Code: "filesystem_unavailable", Level: "unknown"})
	} else {
		const gib int64 = 1 << 30
		warning, critical := int64(20*gib), int64(10*gib)
		if in.Runtime != nil && in.Runtime.BudgetPolicy != nil {
			warning = in.Runtime.BudgetPolicy.MinFreeBytes
			critical = in.Runtime.BudgetPolicy.CriticalFreeBytes
		}
		if *fs.AvailableBytes <= critical {
			out = append(out, CapacityAlert{Code: "free_space_critical", Level: "critical"})
		} else if *fs.AvailableBytes <= warning {
			out = append(out, CapacityAlert{Code: "free_space_warning", Level: "warning"})
		}
		used := float64(*fs.UsedBytes) / float64(*fs.TotalBytes)
		if used >= .90 {
			out = append(out, CapacityAlert{Code: "usage_critical", Level: "critical"})
		} else if used >= .80 {
			out = append(out, CapacityAlert{Code: "usage_warning", Level: "warning"})
		}
	}
	for _, r := range in.Resources {
		if r.State != "observed" {
			out = append(out, CapacityAlert{Code: "source_unavailable", Level: "unknown", ResourceID: r.ID})
		}
	}
	if in.Database != nil && in.Database.State != "observed" {
		out = append(out, CapacityAlert{Code: "database_partial", Level: "unknown"})
	}
	if in.Runtime != nil {
		if in.Runtime.Problem != "" {
			out = append(out, CapacityAlert{Code: "runtime_unavailable", Level: "unknown"})
		}
		if b := in.Runtime.Budget; b != nil && b.MaxUsedBytes > 0 && b.UsedBytes+b.ReservedBytes > b.MaxUsedBytes {
			out = append(out, CapacityAlert{Code: "host_budget_exceeded", Level: "warning"})
		}
		if l := in.Runtime.Lifecycle; l != nil {
			seen := map[string]bool{}
			for i := len(l.Runs) - 1; i >= 0; i-- {
				r := l.Runs[i]
				if seen[r.SourceID] {
					continue
				}
				seen[r.SourceID] = true
				recentProblem := false
				if r.Problem != "" && r.FinishedAt != nil {
					age := in.ObservedAt.Sub(*r.FinishedAt)
					recentProblem = age >= 0 && age < 30*time.Minute
				}
				if (r.State == "unconfirmed" && r.AcknowledgedAt == nil) || recentProblem {
					out = append(out, CapacityAlert{Code: "lifecycle_requires_attention", Level: "warning", ResourceID: r.SourceID})
				}
			}
		}
		if d := in.Runtime.LogDelivery; d != nil && d.LastError != "" {
			out = append(out, CapacityAlert{Code: "log_delivery_degraded", Level: "warning"})
		}
	}
	return out
}
