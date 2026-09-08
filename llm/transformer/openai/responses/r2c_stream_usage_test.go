package responses

import (
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/streams"
)

// fork(R2C): 权威 usage 回归测试。
//
// 复现自真实上游抓包（2026-09-08）：
//   - modelscope Qwen3.8-Flash-Next：每个块都带零值 usage 占位，真值在 finish_reason
//     之后的独立块（choices 为空）。
//   - workbuddy hy3：中间块 usage 为 null，真值与 finish_reason 同块到达。
//
// 契约：response.completed 必须携带"最后一条非零 usage"，且终止事件恰好发出一次。

const r2cUsageProbeID = "resp_r2c_usage_probe"

// r2cChunk 造一个 chat 流式块：可选文本增量、finish_reason、usage（nil 表示该块不带 usage 字段）。
func r2cChunk(text, finishReason string, usage *llm.Usage) *llm.Response {
	chunk := &llm.Response{
		Object:  "chat.completion.chunk",
		ID:      r2cUsageProbeID,
		Created: 1700000000,
		Model:   "probe",
	}

	if text != "" || finishReason != "" {
		delta := &llm.Message{Role: "assistant"}
		if text != "" {
			delta.Content = llm.MessageContent{Content: lo.ToPtr(text)}
		}

		var finish *string
		if finishReason != "" {
			finish = lo.ToPtr(finishReason)
		}

		chunk.Choices = []llm.Choice{{Index: 0, Delta: delta, FinishReason: finish}}
	}

	chunk.Usage = usage

	return chunk
}

func r2cTerminal(t *testing.T, chunks []*llm.Response) (*Response, StreamEventType, int) {
	t.Helper()

	trans := NewInboundTransformer()

	stream, err := trans.TransformStream(t.Context(), streams.SliceStream(chunks))
	require.NoError(t, err)

	var (
		terminal  *Response
		kind      StreamEventType
		terminals int
	)

	for stream.Next() {
		var event StreamEvent
		require.NoError(t, json.Unmarshal(stream.Current().Data, &event))

		if isResponsesTerminalEventType(event.Type) && event.Response != nil {
			terminals++
			kind = event.Type
			terminal = event.Response
		}
	}
	require.NoError(t, stream.Err())

	return terminal, kind, terminals
}

func TestR2CInboundStreamTerminalCarriesAuthoritativeUsage(t *testing.T) {
	tests := []struct {
		name   string
		chunks []*llm.Response
		// wantUsage 为 nil 时只要求"没有谎报非零值"（缺失或全零都可接受）。
		wantUsage *Usage
	}{
		{
			name: "modelscope 每块占位零值，真值在 finish 之后",
			chunks: []*llm.Response{
				r2cChunk("", "", &llm.Usage{}),
				r2cChunk("你好", "", &llm.Usage{}),
				r2cChunk("呀", "", &llm.Usage{}),
				r2cChunk("", "stop", &llm.Usage{}),
				r2cChunk("", "", &llm.Usage{PromptTokens: 66, CompletionTokens: 33, TotalTokens: 99}),
				llm.DoneResponse,
			},
			wantUsage: &Usage{InputTokens: 66, OutputTokens: 33, TotalTokens: 99},
		},
		{
			name: "workbuddy 真值与 finish 同块（现状不得回退）",
			chunks: []*llm.Response{
				r2cChunk("你好", "", nil),
				r2cChunk("呀", "", nil),
				r2cChunk("", "stop", &llm.Usage{
					PromptTokens: 18, CompletionTokens: 25, TotalTokens: 43,
					CompletionTokensDetails: &llm.CompletionTokensDetails{ReasoningTokens: 21},
				}),
				llm.DoneResponse,
			},
			wantUsage: &Usage{InputTokens: 18, OutputTokens: 25, TotalTokens: 43},
		},
		{
			name: "累计 usage 逐块更新，取最后一条非零",
			chunks: []*llm.Response{
				r2cChunk("你好", "", &llm.Usage{PromptTokens: 20, CompletionTokens: 8, TotalTokens: 28}),
				r2cChunk("", "stop", &llm.Usage{PromptTokens: 30, CompletionTokens: 12, TotalTokens: 42}),
				llm.DoneResponse,
			},
			wantUsage: &Usage{InputTokens: 30, OutputTokens: 12, TotalTokens: 42},
		},
		{
			name: "全程只有占位零值，仍须正常收尾且不谎报",
			chunks: []*llm.Response{
				r2cChunk("你好", "", &llm.Usage{}),
				r2cChunk("", "stop", &llm.Usage{}),
				llm.DoneResponse,
			},
			wantUsage: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			terminal, kind, terminals := r2cTerminal(t, tc.chunks)

			require.Equal(t, 1, terminals, "终止事件必须且只能发出一次")
			require.NotNil(t, terminal)
			require.Equal(t, StreamEventTypeResponseCompleted, kind)

			if tc.wantUsage == nil {
				if u := terminal.Usage; u != nil {
					require.Zero(t, u.InputTokens, "无权威 usage 时不得上报非零输入")
					require.Zero(t, u.OutputTokens, "无权威 usage 时不得上报非零输出")
				}
				return
			}

			require.NotNil(t, terminal.Usage, "终止事件缺少 usage")
			require.Equal(t, tc.wantUsage.InputTokens, terminal.Usage.InputTokens)
			require.Equal(t, tc.wantUsage.OutputTokens, terminal.Usage.OutputTokens)
			require.Equal(t, tc.wantUsage.TotalTokens, terminal.Usage.TotalTokens)
		})
	}
}

func TestR2CInboundStreamTerminalKeepsReasoningTokenDetail(t *testing.T) {
	terminal, _, _ := r2cTerminal(t, []*llm.Response{
		r2cChunk("你好", "", nil),
		r2cChunk("", "stop", &llm.Usage{
			PromptTokens: 18, CompletionTokens: 25, TotalTokens: 43,
			CompletionTokensDetails: &llm.CompletionTokensDetails{ReasoningTokens: 21},
		}),
		llm.DoneResponse,
	})
	require.NotNil(t, terminal.Usage)
	require.Equal(t, int64(21), terminal.Usage.OutputTokenDetails.ReasoningTokens)
}
