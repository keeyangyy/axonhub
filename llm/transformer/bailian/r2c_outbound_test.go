package bailian

import (
	"encoding/json"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/auth"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/streams"
	"github.com/looplj/axonhub/llm/transformer/openai"
	responsesapi "github.com/looplj/axonhub/llm/transformer/openai/responses"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBailianResponsesToolLifecycle_StreamsThroughWrapper(t *testing.T) {
	outbound, err := NewOutboundTransformerWithConfig(&Config{
		BaseURL:        "https://example.com",
		APIKeyProvider: auth.NewStaticKeyProvider("test-key"),
	})
	require.NoError(t, err)
	bailian := outbound.(*OutboundTransformer)
	require.True(t, bailian.ResponsesRequestCapabilities(&llm.Request{}).ChatToolLifecycle)
	require.False(t, bailian.ResponsesRequestCapabilities(&llm.Request{RequestType: llm.RequestTypeCompact}).ChatToolLifecycle)

	request := &llm.Request{
		Model:     "qwen-max",
		APIFormat: llm.APIFormatOpenAIResponse,
		Stream:    lo.ToPtr(true),
		Messages:  []llm.Message{{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("patch")}}},
		Tools: []llm.Tool{{
			Type: llm.ToolTypeResponsesCustomTool,
			ResponseCustomTool: &llm.ResponseCustomTool{
				Name: "apply_patch", Description: "Apply patch text",
			},
		}},
	}
	httpRequest, err := bailian.TransformRequest(t.Context(), request)
	require.NoError(t, err)
	require.Contains(t, httpRequest.TransformerMetadata, "openai_responses_chat_tool_mappings")
	require.Contains(t, httpRequest.TransformerMetadata, "openai_responses_chat_tool_catalog")
	require.Equal(t, llm.ToolTypeResponsesCustomTool, request.Tools[0].Type)

	var wire openai.Request
	require.NoError(t, json.Unmarshal(httpRequest.Body, &wire))
	require.Len(t, wire.Tools, 1)
	require.Equal(t, llm.ToolTypeFunction, wire.Tools[0].Type)
	require.Equal(t, "apply_patch", wire.Tools[0].Function.Name)
	require.JSONEq(t, `{"type":"object","properties":{"input":{"type":"string","description":"Exact raw custom-tool input. Escape quotes, backslashes, and control characters only as required for the outer JSON string; do not add another object or serialization layer."}},"required":["input"],"additionalProperties":false}`, string(wire.Tools[0].Function.Parameters))

	providerStream, err := bailian.TransformStream(t.Context(), httpRequest, streams.SliceStream([]*httpclient.StreamEvent{
		{Data: []byte(`{"id":"chatcmpl_1","object":"chat.completion.chunk","model":"qwen-max","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"apply_patch","arguments":"{\"input\":\"patch\"}"}}]},"finish_reason":"tool_calls"}]}`)},
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
