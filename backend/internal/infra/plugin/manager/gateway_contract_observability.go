package manager

import (
	"context"
	"expvar"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

var (
	pluginGatewayContractValidGauge = expvar.NewMap("plugin_gateway_contract_valid")
	pluginGatewayContractProbeTotal = expvar.NewMap("plugin_gateway_contract_probe_total")
)

func recordGatewayContractValid(pluginID string, valid bool) {
	if pluginID == "" {
		return
	}
	value := new(expvar.Int)
	if valid {
		value.Set(1)
	} else {
		value.Set(0)
	}
	pluginGatewayContractValidGauge.Set(pluginID, value)
}

func recordGatewayContractProbeResult(result string) {
	if result == "" {
		result = "unknown"
	}
	pluginGatewayContractProbeTotal.Add(result, 1)
}

func emitGatewayContractAudit(ctx context.Context, pluginID string, fields map[string]any, message string) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["plugin_id"] = pluginID
	logger.InfoF(ctx, "[gateway_contract] %s fields=%v", message, fields)
}
