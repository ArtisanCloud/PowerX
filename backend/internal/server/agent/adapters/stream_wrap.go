// services/agent/adapters/stream_wrap.go
package adapters

import (
	"errors"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"io"

	acontract "github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	aschemas "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/cloudwego/eino/schema"
)

// 把 ExternalStream 适配为 eino 的 StreamReader[*ExecutionResult]
func toEinoStream(ext acontract.ExternalStream) *schema.StreamReader[*aschemas.ExecutionResult] {
	sr, sw := agent.NewResultPipe(8) // 缓冲随意给个 8

	go func() {
		defer sw.Close() // 通知接收方 EOF
		defer ext.Close()

		for {
			chunk, err := ext.Recv()
			if err != nil {
				// 正常结束：直接返回（Pipe 会给对端 io.EOF）
				if errors.Is(err, io.EOF) {
					return
				}
				// 异常：把错误也送出去一次，随后结束
				_ = sw.Send(nil, err)
				return
			}
			// 正常增量
			if closed := sw.Send(chunk, nil); closed {
				return
			}
		}
	}()

	return sr
}
