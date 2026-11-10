package capability_registry

import (
	"testing"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/stretchr/testify/require"
)

func TestServiceTicketContractRejectsFreeProxyBinding(t *testing.T) {
	valid := models.ProtocolBinding{Channel: "rest", Endpoint: "/api/v1/service/messages", Method: "POST", AuthType: PluginServiceAuthType, ActorContext: "service_actor", ResourceScope: "tenant", SchemaRef: "contracts/messages.yaml"}
	require.NoError(t, validateServiceTicketProtocols([]models.ProtocolBinding{valid}))
	for _, alter := range []func(*models.ProtocolBinding){
		func(p *models.ProtocolBinding) { p.ActorContext = "admin_user" },
		func(p *models.ProtocolBinding) { p.ResourceScope = "global" },
		func(p *models.ProtocolBinding) { p.Method = "CONNECT" },
		func(p *models.ProtocolBinding) { p.SchemaRef = "" },
		func(p *models.ProtocolBinding) { p.Endpoint = "https://example.invalid/messages" },
		func(p *models.ProtocolBinding) { p.Endpoint = "/api/v1/{message_id}" },
		func(p *models.ProtocolBinding) { p.Endpoint = "/api/v1/service?tenant=other" },
		func(p *models.ProtocolBinding) { p.Endpoint = "/api/v1/../admin" },
		func(p *models.ProtocolBinding) { p.Endpoint = "//other/service" },
	} {
		p := valid
		alter(&p)
		require.Error(t, validateServiceTicketProtocols([]models.ProtocolBinding{p}))
	}
}
