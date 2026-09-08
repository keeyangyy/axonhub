package responses

import (
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/streams"
)

// r2c_stream_usage.go (fork; R2C-only)
//
// 部分只支持 Chat Completions 的上游（实测 modelscope）会在**每个**流式块里塞一份
// prompt/completion/total 全为 0 的 usage 占位，真正的用量在 finish_reason 之后的
// 独立块里才给一次。而 responsesInboundStream 的终止判定是"块带 usage 且已 finish 就
// 立刻发 response.completed 并上锁"（见 inbound_stream.go 的 hasFinished/responseCompleted），
// 于是它在 finish 块上就用占位零值封口，随后的权威 usage 被整块忽略——客户端
// （Codex 家族）拿到 0/0/0，上下文计量恒为 0 且自动压缩永不触发。
//
// 这里只在流上做一件事：把占位 usage 伪装成"该块没带 usage"。真值非零，不受影响；
// finish_reason 原样保留，所以终止判定只是被推迟到权威 usage 到达，或走既有的
// 流结束兜底分支，不会挂起、也不会重复发终止事件。

// isR2CPlaceholderUsage reports an all-zero usage payload that some Chat upstreams
// stamp on every stream chunk as a placeholder rather than as authoritative counts.
func isR2CPlaceholderUsage(usage *llm.Usage) bool {
	return usage != nil &&
		usage.PromptTokens == 0 &&
		usage.CompletionTokens == 0 &&
		usage.TotalTokens == 0
}

// r2cAuthoritativeUsageStream hides placeholder usage chunks so the Responses terminal
// event is sealed only by an authoritative usage chunk (or by the existing stream-end
// fallback). Chunks are copied on write: the original stream objects stay intact for
// accounting, and llm.DoneResponse keeps its identity for consumers that compare it.
type r2cAuthoritativeUsageStream struct {
	inner   streams.Stream[*llm.Response]
	current *llm.Response
}

func newR2CAuthoritativeUsageStream(
	inner streams.Stream[*llm.Response],
) streams.Stream[*llm.Response] {
	return &r2cAuthoritativeUsageStream{inner: inner}
}

func (s *r2cAuthoritativeUsageStream) Next() bool {
	if !s.inner.Next() {
		s.current = nil
		return false
	}

	chunk := s.inner.Current()
	if isR2CPlaceholderUsage(chunk.Usage) {
		stripped := *chunk
		stripped.Usage = nil
		s.current = &stripped
	} else {
		s.current = chunk
	}

	return true
}

func (s *r2cAuthoritativeUsageStream) Current() *llm.Response { return s.current }

func (s *r2cAuthoritativeUsageStream) Err() error { return s.inner.Err() }

func (s *r2cAuthoritativeUsageStream) Close() error { return s.inner.Close() }
