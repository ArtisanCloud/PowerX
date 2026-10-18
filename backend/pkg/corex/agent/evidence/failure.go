package evidence

import (
	"errors"
	"strings"
)

type stagedFailure interface{ EvidenceFailure() ToolFailure }

func (e *ToolEvidenceError) EvidenceFailure() ToolFailure { return e.ToolFailure }

// FailureDetails 只识别平台稳定错误，不把 provider 原始响应当作公开错误码。
func FailureDetails(err error) (ToolFailure, bool) {
	var staged stagedFailure
	if errors.As(err, &staged) {
		return staged.EvidenceFailure(), true
	}
	if err != nil {
		for code, stage := range map[string]string{"evidence.storage_unavailable": "evidence_validation", "evidence.archive_invalid": "evidence_validation", "agent.response_contract_invalid": "final_validation"} {
			if strings.HasPrefix(err.Error(), code) {
				return ToolFailure{Code: code, Stage: stage}, true
			}
		}
	}
	return ToolFailure{}, false
}
