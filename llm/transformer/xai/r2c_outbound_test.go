package xai

import (
	"encoding/json"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/auth"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/streams"
	responsesapi "github.com/looplj/axonhub/llm/transformer/openai/responses"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOutboundTransformer_ResponsesToolLifecycle_StreamsThroughWrapper(t *testing.T) {
	outbound, err := NewOutboundTransformerWithConfig(&Config{
		BaseURL:        DefaultBaseURL,
		APIKeyProvider: auth.NewStaticKeyProvider("test-key"),
	})
	require.NoError(t, err)
	xai := outbound.(*OutboundTransformer)
	require.True(t, xai.ResponsesRequestCapabilities(&llm.Request{}).ChatToolLifecycle)
	require.False(t, xai.ResponsesRequestCapabilities(&llm.Request{RequestType: llm.RequestTypeCompact}).ChatToolLifecycle)

	request := &llm.Request{
		Model:     "grok-code-fast-1",
		APIFormat: llm.APIFormatOpenAIResponse,
		Stream:    lo.ToPtr(true),
		Messages:  []llm.Message{{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("patch")}}},
		Tools: []llm.Tool{{
			Type:               llm.ToolTypeResponsesCustomTool,
			ResponseCustomTool: &llm.ResponseCustomTool{Name: "apply_patch"},
		}},
	}
	httpRequest, err := xai.TransformRequest(t.Context(), request)
	require.NoError(t, err)
	require.Contains(t, httpRequest.TransformerMetadata, "openai_responses_chat_tool_mappings")
	require.Contains(t, httpRequest.TransformerMetadata, "openai_responses_chat_tool_catalog")
	require.Equal(t, llm.ToolTypeResponsesCustomTool, request.Tools[0].Type)

	var wire map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(httpRequest.Body, &wire))
	var tools []struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	require.NoError(t, json.Unmarshal(wire["tools"], &tools))
	require.Len(t, tools, 1)
	require.Equal(t, llm.ToolTypeFunction, tools[0].Type)
	require.Equal(t, "apply_patch", tools[0].Function.Name)

	providerStream, err := xai.TransformStream(t.Context(), httpRequest, streams.SliceStream([]*httpclient.StreamEvent{
		{Data: []byte(`{"id":"chatcmpl_1","object":"chat.completion.chunk","model":"grok-code-fast-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"apply_patch","arguments":"{\"input\":\"patch\"}"}}]},"finish_reason":"tool_calls"}]}`)},
		{Data: []byte(`[DONE]`)},
	}))
	require.NoError(t, err)
	stream, err := responsesapi.NewInboundTransformer().TransformStream(t.Context(), providerStream)
	require.NoError(t, err)

	var customDone *responsesapi.Item
	for stream.Next() {
		var event responsesapi.StreamEvent
		require.NoError(t, json.Unmarshal(stream.Current().Data, &event))
		if event.Type == responsesapi.StreamEventTypeOutputItemDone && event.Item != nil && event.Item.Type == "custom_tool_call" {
			customDone = event.Item
		}
	}
	require.NoError(t, stream.Err())
	require.NotNil(t, customDone)
	require.Equal(t, "apply_patch", customDone.Name)
	require.Equal(t, "patch", lo.FromPtr(customDone.Input))
}

func TestOutboundTransformer_TransformRequest_DoesNotMutateRetryInput(t *testing.T) {
	outbound, err := NewOutboundTransformerWithConfig(&Config{
		BaseURL:        DefaultBaseURL,
		APIKeyProvider: auth.NewStaticKeyProvider("test-key"),
	})
	require.NoError(t, err)

	stop := &llm.Stop{Stop: lo.ToPtr("END")}
	request := &llm.Request{
		Model:            "grok-4",
		Messages:         []llm.Message{{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("hello")}}},
		ReasoningEffort:  "high",
		PresencePenalty:  lo.ToPtr(0.5),
		FrequencyPenalty: lo.ToPtr(0.25),
		Stop:             stop,
	}

	httpRequest, err := outbound.TransformRequest(t.Context(), request)
	require.NoError(t, err)
	var wire map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(httpRequest.Body, &wire))
	require.NotContains(t, wire, "reasoning_effort")
	require.NotContains(t, wire, "presence_penalty")
	require.NotContains(t, wire, "frequency_penalty")
	require.NotContains(t, wire, "stop")

	require.Equal(t, "high", request.ReasoningEffort)
	require.Equal(t, 0.5, lo.FromPtr(request.PresencePenalty))
	require.Equal(t, 0.25, lo.FromPtr(request.FrequencyPenalty))
	require.Same(t, stop, request.Stop)
	require.Equal(t, "END", lo.FromPtr(request.Stop.Stop))
}
