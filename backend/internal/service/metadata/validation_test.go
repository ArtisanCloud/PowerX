package metadata

import "testing"

func TestValidateMachineIdentifier(t *testing.T) {
	valid := []string{"corex.customer.level", "customer_level", "customer-level"}
	for _, value := range valid {
		if err := ValidateMachineIdentifier(value); err != nil {
			t.Fatalf("expected valid identifier %q: %v", value, err)
		}
	}
	invalid := []string{"", "客户等级", "Customer Level", "550e8400-e29b-41d4-a716-446655440000"}
	for _, value := range invalid {
		if err := ValidateMachineIdentifier(value); err == nil {
			t.Fatalf("expected invalid identifier %q", value)
		}
	}
}

func TestValidateRequiredI18n(t *testing.T) {
	if err := ValidateRequiredI18n(map[string]string{"zh-CN": "客户等级"}, "zh-CN"); err != nil {
		t.Fatalf("expected zh-CN label to pass: %v", err)
	}
	if err := ValidateRequiredI18n(map[string]string{"en-US": "Level"}, "zh-CN"); err == nil {
		t.Fatalf("expected missing zh-CN label to fail")
	}
}
