package capability_registry

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
)

func encodeEnvironmentPolicies(policies map[string]EnvironmentPolicy) (datatypes.JSON, error) {
	if len(policies) == 0 {
		return datatypes.JSON([]byte("{}")), nil
	}
	raw, err := json.Marshal(policies)
	if err != nil {
		return datatypes.JSON{}, err
	}
	return datatypes.JSON(raw), nil
}

func encodeToolGrantIDs(ids []string) (datatypes.JSON, error) {
	if len(ids) == 0 {
		return datatypes.JSON([]byte("[]")), nil
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return datatypes.JSON{}, err
	}
	return datatypes.JSON(raw), nil
}

func encodeMap(v interface{}, empty datatypes.JSON) (datatypes.JSON, error) {
	if v == nil {
		return empty, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON{}, err
	}
	return datatypes.JSON(raw), nil
}

func encodeRoutingPolicy(policy RoutingPolicy) (*models.RoutingPolicy, error) {
	tenantStrategies, err := encodeMap(policy.TenantStrategies, datatypes.JSON([]byte("{}")))
	if err != nil {
		return nil, err
	}
	var rateLimit datatypes.JSON
	if policy.RateLimit != nil {
		rateLimit, err = encodeMap(policy.RateLimit, datatypes.JSON([]byte("{}")))
		if err != nil {
			return nil, err
		}
	} else {
		rateLimit = datatypes.JSON([]byte("{}"))
	}
	fallbackSeq, err := encodeMap(policy.FallbackSequence, datatypes.JSON([]byte("[]")))
	if err != nil {
		return nil, err
	}
	stickyKeys, err := encodeMap(policy.StickyKeys, datatypes.JSON([]byte("[]")))
	if err != nil {
		return nil, err
	}

	model := &models.RoutingPolicy{
		Strategy:         policy.Strategy,
		TenantStrategies: tenantStrategies,
		RateLimit:        rateLimit,
		FallbackSequence: fallbackSeq,
		CooldownSeconds:  policy.CooldownSeconds,
		StickyKeys:       stickyKeys,
	}
	if policy.ID != uuid.Nil {
		model.UUID = policy.ID
	}
	return model, nil
}

func encodeFallbackPlan(plan *FallbackPlan, primaryCapabilityID string) (*models.FallbackPlan, error) {
	if plan == nil {
		return nil, nil
	}
	fallbackTargets, err := encodeMap(plan.FallbackTargets, datatypes.JSON([]byte("[]")))
	if err != nil {
		return nil, err
	}
	staticResponse, err := encodeMap(plan.StaticResponse, datatypes.JSON([]byte("{}")))
	if err != nil {
		return nil, err
	}
	triggerConditions, err := encodeMap(plan.TriggerConditions, datatypes.JSON([]byte("{}")))
	if err != nil {
		return nil, err
	}
	model := &models.FallbackPlan{
		PrimaryCapabilityID: primaryCapabilityID,
		FallbackTargets:     fallbackTargets,
		StaticResponse:      staticResponse,
		TriggerConditions:   triggerConditions,
		NotificationChannel: plan.NotificationChannel,
	}
	if plan.ID != uuid.Nil {
		model.UUID = plan.ID
	}
	return model, nil
}

func encodeAdapterEndpoints(reg Registration, tenantUUID string, registrationID uint64) ([]models.AdapterEndpoint, error) {
	adapters := make([]models.AdapterEndpoint, 0, len(reg.Adapters))
	seen := make(map[string]struct{}, len(reg.Adapters))
	canonicalTenant := canonicalTenantUUID(tenantUUID)
	for _, adapter := range reg.Adapters {
		if _, ok := seen[adapter.AdapterID]; ok {
			return nil, ErrDuplicateAdapterID
		}
		seen[adapter.AdapterID] = struct{}{}

		labels, err := encodeMap(adapter.Labels, datatypes.JSON([]byte("{}")))
		if err != nil {
			return nil, err
		}
		visibility, err := encodeMap(adapter.Visibility, datatypes.JSON([]byte("{}")))
		if err != nil {
			return nil, err
		}
		record := models.AdapterEndpoint{
			RegistrationID: registrationID,
			CapabilityID:   reg.CapabilityID,
			TenantUUID:     canonicalTenant,
			AdapterID:      adapter.AdapterID,
			TransportType:  adapter.TransportType,
			Endpoint:       adapter.Endpoint,
			ServiceRef:     adapter.ServiceRef,
			Weight:         adapter.Weight,
			TimeoutMS:      adapter.TimeoutMS,
			MaxConcurrency: adapter.MaxConcurrency,
			Labels:         labels,
			Visibility:     visibility,
			IsActive:       adapter.IsActive,
		}
		if adapter.HealthPolicyID != nil {
			record.HealthPolicyID = adapter.HealthPolicyID
		}
		adapters = append(adapters, record)
	}
	return adapters, nil
}

func decodeRegistration(model models.CapabilityRegistration) (Registration, error) {
	reg := Registration{
		CapabilityID:  model.CapabilityID,
		TenantUUID:    model.TenantUUID,
		ContractRef:   model.ContractRef,
		Status:        model.Status,
		Version:       model.Version,
		CreatedAt:     model.CreatedAt,
		UpdatedAt:     model.UpdatedAt,
		UpdatedBy:     model.UpdatedBy,
		PublishedAt:   model.PublishedAt,
		DisableReason: model.DisableReason,
	}

	if len(model.EnvironmentPolicies) > 0 {
		if err := json.Unmarshal(model.EnvironmentPolicies, &reg.EnvironmentPolicies); err != nil {
			return Registration{}, err
		}
	} else {
		reg.EnvironmentPolicies = map[string]EnvironmentPolicy{}
	}
	if len(model.ToolGrantIDs) > 0 {
		if err := json.Unmarshal(model.ToolGrantIDs, &reg.ToolGrantIDs); err != nil {
			return Registration{}, err
		}
	} else {
		reg.ToolGrantIDs = []string{}
	}

	if model.RoutingPolicy != nil {
		var tenantStrategies map[string]string
		if len(model.RoutingPolicy.TenantStrategies) > 0 {
			if err := json.Unmarshal(model.RoutingPolicy.TenantStrategies, &tenantStrategies); err != nil {
				return Registration{}, err
			}
		}
		var rateLimit *RateLimit
		if len(model.RoutingPolicy.RateLimit) > 0 {
			var parsed RateLimit
			if err := json.Unmarshal(model.RoutingPolicy.RateLimit, &parsed); err != nil {
				return Registration{}, err
			}
			rateLimit = &parsed
		}
		var fallbackSeq []string
		if len(model.RoutingPolicy.FallbackSequence) > 0 {
			if err := json.Unmarshal(model.RoutingPolicy.FallbackSequence, &fallbackSeq); err != nil {
				return Registration{}, err
			}
		}
		var stickyKeys []string
		if len(model.RoutingPolicy.StickyKeys) > 0 {
			if err := json.Unmarshal(model.RoutingPolicy.StickyKeys, &stickyKeys); err != nil {
				return Registration{}, err
			}
		}
		reg.RoutingPolicy = RoutingPolicy{
			ID:               model.RoutingPolicy.UUID,
			Strategy:         model.RoutingPolicy.Strategy,
			TenantStrategies: tenantStrategies,
			RateLimit:        rateLimit,
			FallbackSequence: fallbackSeq,
			CooldownSeconds:  model.RoutingPolicy.CooldownSeconds,
			StickyKeys:       stickyKeys,
			LastUpdatedAt:    model.RoutingPolicy.UpdatedAt,
		}
	}

	if model.FallbackPlan != nil {
		var targets []string
		if len(model.FallbackPlan.FallbackTargets) > 0 {
			if err := json.Unmarshal(model.FallbackPlan.FallbackTargets, &targets); err != nil {
				return Registration{}, err
			}
		}
		var static *StaticResponse
		if len(model.FallbackPlan.StaticResponse) > 0 && string(model.FallbackPlan.StaticResponse) != "{}" {
			var parsed StaticResponse
			if err := json.Unmarshal(model.FallbackPlan.StaticResponse, &parsed); err != nil {
				return Registration{}, err
			}
			static = &parsed
		}
		var triggers map[string]interface{}
		if len(model.FallbackPlan.TriggerConditions) > 0 {
			if err := json.Unmarshal(model.FallbackPlan.TriggerConditions, &triggers); err != nil {
				return Registration{}, err
			}
		}
		reg.FallbackPlan = &FallbackPlan{
			ID:                  model.FallbackPlan.UUID,
			PrimaryCapabilityID: model.FallbackPlan.PrimaryCapabilityID,
			FallbackTargets:     targets,
			StaticResponse:      static,
			TriggerConditions:   triggers,
			NotificationChannel: model.FallbackPlan.NotificationChannel,
		}
	}

	if len(model.Adapters) > 0 {
		adapters := make([]AdapterEndpoint, 0, len(model.Adapters))
		for _, item := range model.Adapters {
			var labels map[string]string
			if len(item.Labels) > 0 {
				if err := json.Unmarshal(item.Labels, &labels); err != nil {
					return Registration{}, err
				}
			}
			var visibility VisibilityPolicy
			if len(item.Visibility) > 0 {
				if err := json.Unmarshal(item.Visibility, &visibility); err != nil {
					return Registration{}, err
				}
			}
			adapters = append(adapters, AdapterEndpoint{
				AdapterID:      item.AdapterID,
				TransportType:  item.TransportType,
				Endpoint:       item.Endpoint,
				ServiceRef:     item.ServiceRef,
				Weight:         item.Weight,
				TimeoutMS:      item.TimeoutMS,
				MaxConcurrency: item.MaxConcurrency,
				Labels:         labels,
				Visibility:     visibility,
				HealthPolicyID: item.HealthPolicyID,
				IsActive:       item.IsActive,
			})
		}
		reg.Adapters = adapters
	} else {
		reg.Adapters = []AdapterEndpoint{}
	}

	return reg, nil
}

func decodeRegistrations(list []models.CapabilityRegistration) ([]Registration, error) {
	result := make([]Registration, 0, len(list))
	for _, item := range list {
		reg, err := decodeRegistration(item)
		if err != nil {
			return nil, err
		}
		result = append(result, reg)
	}
	return result, nil
}
