package shared

import (
	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"time"
)

const aiResponseGrace = 10 * time.Second

// resolveAIMultimodalHTTPTimeout 为配置中的排队和推理各保留完整期限，
// 再留出错误回包窗口。显式配置更长的代理期限仍保留；普通 HTTP 请求不受影响。
func resolveAIMultimodalHTTPTimeout(opts CapabilityRegistryOptions) time.Duration {
	defaults := agentconfig.AIConfig{}
	defaults.SetDefaults()
	request := opts.LLMRequestTimeout
	if request <= 0 {
		request = defaults.Defaults.LLM.RequestTimeout
	}
	queue := opts.LLMQueueWaitTimeout
	if queue <= 0 {
		queue = defaults.Runtime.QueueWaitTimeout
	}
	// 不让极端配置的 duration 相加溢出成非正数，从而意外关闭 HTTP 超时。
	const longest = time.Duration(1<<63 - 1)
	if queue > longest-request || queue+request > longest-aiResponseGrace {
		return longest
	}
	required := queue + request + aiResponseGrace
	if opts.AIMultimodalHTTPTimeout > required {
		return opts.AIMultimodalHTTPTimeout
	}
	return required
}

// resolveAIHandlerTimeout 保留非 LLM Handler 原有的配置期限。
// 代理的额外排队／回包时间不能反向扩大 Embedding 等业务处理时限。
func resolveAIHandlerTimeout(opts CapabilityRegistryOptions) time.Duration {
	if opts.AIMultimodalHTTPTimeout > 0 {
		return opts.AIMultimodalHTTPTimeout
	}
	defaults := agentconfig.AIConfig{}
	defaults.SetDefaults()
	return defaults.Defaults.LLM.RequestTimeout + aiResponseGrace
}
