package seed

import (
	_ "embed"
	"encoding/json"
)

//go:embed locales/marketing_calculation_policy.json
var nativeMarketingCalculationPolicyJSON []byte

//go:embed locales/marketing_review_policy.json
var nativeMarketingReviewPolicyJSON []byte

// 这是发布到 Skill Revision 的业务配置，不是 Runtime 分派或计算实现。
func nativeMarketingCalculationPolicy() (map[string]any, error) {
	var policy map[string]any
	err := json.Unmarshal(nativeMarketingCalculationPolicyJSON, &policy)
	return policy, err
}

func nativeMarketingReviewPolicy() (map[string]any, error) {
	var policy map[string]any
	err := json.Unmarshal(nativeMarketingReviewPolicyJSON, &policy)
	return policy, err
}
