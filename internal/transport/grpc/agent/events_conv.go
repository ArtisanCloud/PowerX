// internal/server/agent/transport/grpc/events_conv.go
package agentgrpc

import (
	"encoding/json"
	agentv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"google.golang.org/protobuf/types/known/structpb"
)

// WS -> gRPC
func ToStreamEvent(env dto.WSEnvelope) (*agentv1.StreamEvent, error) {
	var m map[string]any
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &m); err != nil {
			return nil, err
		}
	}
	st, _ := structpb.NewStruct(m)
	return &agentv1.StreamEvent{
		Type:      env.Type,
		Data:      st,
		Timestamp: env.Timestamp,
	}, nil
}

// gRPC -> WS
func ToWSEnvelope(ev *agentv1.StreamEvent) (dto.WSEnvelope, error) {
	var raw json.RawMessage
	if ev.GetData() != nil {
		b, err := json.Marshal(ev.GetData().AsMap())
		if err != nil {
			return dto.WSEnvelope{}, err
		}
		raw = b
	}
	return dto.WSEnvelope{
		Type:      ev.GetType(),
		Data:      raw,
		Timestamp: ev.GetTimestamp(),
	}, nil
}
