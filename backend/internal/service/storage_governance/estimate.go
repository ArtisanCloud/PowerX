package storagegov

import (
	"errors"
	"math"
)

// ErrEstimateInvalid 表示测算参数非法，不可向客户端回传未校验的原始输入。
var ErrEstimateInvalid = errors.New("容量测算参数无效")

const maxSafeBytes int64 = 1<<53 - 1 // JSON／浏览器整数共同安全范围。

// CalculateEstimate 用精确整数向上取整，不因浮点误差少预留一个字节。
func CalculateEstimate(in EstimateInput) (Estimate, error) {
	out := Estimate{State: "sampling_required", Basis: "unavailable", SafetyFactor: 1.3}
	if in.ResourceID != "" || in.RetentionMode != "" || in.RetentionCount != 0 {
		return out, ErrEstimateInvalid
	}
	if in.TargetDays < 1 || in.TargetDays > 3650 || in.MaxBytes < 1 || in.MaxBytes > maxSafeBytes || in.ProtectedBytes < 0 || in.ProtectedBytes > maxSafeBytes {
		return out, ErrEstimateInvalid
	}
	if in.DailyIncrementBytes == nil {
		return out, nil
	}
	rate := *in.DailyIncrementBytes
	if rate <= 0 || rate > maxSafeBytes || rate > (maxSafeBytes-in.ProtectedBytes)*10/(int64(in.TargetDays)*13) {
		return out, ErrEstimateInvalid
	}
	// 上界校验后乘积不会溢出 int64。
	required := (rate*int64(in.TargetDays)*13+9)/10 + in.ProtectedBytes
	if required > maxSafeBytes {
		return out, ErrEstimateInvalid
	}
	deficit := max(int64(0), required-in.MaxBytes)
	supported := float64(max(int64(0), in.MaxBytes-in.ProtectedBytes)) / (float64(rate) * 1.3)
	supported = math.Round(supported*10) / 10
	out.State, out.Basis = "budget_feasible", "manual_assumption"
	if deficit > 0 {
		out.State = "budget_insufficient"
	}
	out.RequiredBytes, out.DeficitBytes, out.SupportedDays = &required, &deficit, &supported
	// 估算可行与可启用分开；只读阶段永远不能启用。
	return out, nil
}

// EstimateResource 在可信登记范围内计算；按份数／引用保护数据不套按天增量公式。
func (s *Service) EstimateResource(in EstimateInput) (Estimate, error) {
	out := Estimate{State: "sampling_required", Basis: "unavailable", SafetyFactor: 1.3}
	var source Resource
	found := false
	for _, r := range s.opts.Resources {
		if r.ID == in.ResourceID {
			source = r
			found = true
			break
		}
	}
	if !found || in.MaxBytes < 1 || in.MaxBytes > maxSafeBytes || in.ProtectedBytes < 0 || in.ProtectedBytes > maxSafeBytes {
		return out, ErrEstimateInvalid
	}
	policy := ResourcePolicy{RetentionMode: in.RetentionMode, TargetDays: in.TargetDays, RetentionCount: in.RetentionCount}
	if in.RetentionMode == "" || !validResourcePolicy(policy, source) {
		return out, ErrEstimateInvalid
	}
	ref := refFor(source)
	out.Resource = &ref
	out.RetentionMode = in.RetentionMode
	out.RetentionCount = in.RetentionCount
	if in.RetentionMode == "count" || in.RetentionMode == "protected" {
		if in.DailyIncrementBytes != nil {
			return out, ErrEstimateInvalid
		}
		out.State = "artifact_inventory_required"
		if in.RetentionMode == "protected" {
			out.State = "retention_not_applicable"
		}
		return out, nil
	}
	in.ResourceID = ""
	in.RetentionMode = ""
	in.RetentionCount = 0
	calculated, err := CalculateEstimate(in)
	calculated.Resource = &ref
	calculated.RetentionMode = "days"
	return calculated, err
}
