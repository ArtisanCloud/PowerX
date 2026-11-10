package capability_registry

import (
	"context"
	"errors"
	"io"
)

type CoreCapabilityMux struct {
	invokers []CoreCapabilityInvoker
}

func NewCoreCapabilityMux(invokers ...CoreCapabilityInvoker) *CoreCapabilityMux {
	items := make([]CoreCapabilityInvoker, 0, len(invokers))
	for _, invoker := range invokers {
		if invoker != nil {
			items = append(items, invoker)
		}
	}
	return &CoreCapabilityMux{invokers: items}
}

func (m *CoreCapabilityMux) InvokeCoreCapability(ctx context.Context, in CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if m == nil || len(m.invokers) == 0 {
		return nil, ErrCoreCapabilityNotHandled
	}
	for _, invoker := range m.invokers {
		payload, err := invoker.InvokeCoreCapability(ctx, in)
		if err == nil {
			return payload, nil
		}
		if errors.Is(err, ErrCoreCapabilityNotHandled) {
			continue
		}
		return nil, err
	}
	return nil, ErrCoreCapabilityNotHandled
}

// CoreCapabilityStreamInvoker 仅接收正式固定绑定，不能代理任意 URL。
type CoreCapabilityStreamInvoker interface {
	InvokeCoreCapabilityStream(context.Context, CoreCapabilityInvokeInput) (io.ReadCloser, error)
}

func (m *CoreCapabilityMux) InvokeCoreCapabilityStream(ctx context.Context, in CoreCapabilityInvokeInput) (io.ReadCloser, error) {
	if m == nil {
		return nil, ErrCoreCapabilityNotHandled
	}
	for _, invoker := range m.invokers {
		streamer, ok := invoker.(CoreCapabilityStreamInvoker)
		if !ok {
			continue
		}
		body, err := streamer.InvokeCoreCapabilityStream(ctx, in)
		if errors.Is(err, ErrCoreCapabilityNotHandled) {
			continue
		}
		return body, err
	}
	return nil, ErrCoreCapabilityNotHandled
}
