package pluginx

import (
	"context"
	"time"
)

type ReadyMonitor struct {
	serverHealthUrl string
	isReady         bool
}

func NewReadyMonitor(serverHealthUrl string) *ReadyMonitor {
	return &ReadyMonitor{
		serverHealthUrl: serverHealthUrl,
	}
}

func (m *ReadyMonitor) WaitReady(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// do nothing
			}

			// 监听主机状态, 如果发现就绪, 则退出循环
			if IsServerReady(m.serverHealthUrl) {
				m.isReady = true
				break
			} else {
				m.isReady = false
			}
			time.Sleep(3 * time.Second)
		}
	}()
}

func (m *ReadyMonitor) IsReady() bool {
	return m.isReady
}
