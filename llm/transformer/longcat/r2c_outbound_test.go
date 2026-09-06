package longcat

import (
	"encoding/json"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/transformer/openai"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestOutboundTransformer_ResponsesToolLifecycle_NonStreamRestore(t *testing.T) {
	outbound, err := NewOutboundTransformer("https://api.example.com", "test-key")
	require.NoError(t, err)
	longcat := outbound.(*OutboundTransformer)
	require.True(t, longcat.ResponsesRequestCapabilities(&llm.Request{}).ChatToolLifecycle)
	require.False(t, longcat.ResponsesRequestCapabilities(&llm.Request{RequestType: llm.RequestTypeCompact}).ChatToolLifecycle)

	request := &llm.Request{
		Model:     "LongCat-Flash-Omni-2603",
		APIFormat: llm.APIFormatOpenAIResponse,
		Stream:    lo.ToPtr(false),
		Messages:  []llm.Message{{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("patch")}}},
		Tools: []llm.Tool{{
			Type:               llm.ToolTypeResponsesCustomTool,
			ResponseCustomTool: &llm.ResponseCustomTool{Name: "apply_patch"},
		}},
	}
	httpRequest, err := longcat.TransformRequest(t.Context(), request)
	require.NoError(t, err)
	require.Contains(t, httpRequest.TransformerMetadata, "openai_responses_chat_tool_mappings")
	require.Contains(t, httpRequest.TransformerMetadata, "openai_responses_chat_tool_catalog")
	require.Equal(t, llm.ToolTypeResponsesCustomTool, request.Tools[0].Type)

	var wire openai.Request
	require.NoError(t, json.Unmarshal(httpRequest.Body, &wire))
	require.Len(t, wire.Tools, 1)
	require.Equal(t, llm.ToolTypeFunction, wire.Tools[0].Type)
	require.Equal(t, "apply_patch", wire.Tools[0].Function.Name)

	response, err := longcat.TransformResponse(t.Context(), &httpclient.Response{
		StatusCode: http.StatusOK,
		Request:    httpRequest,
		Body: []byte(`{
			"id":"chatcmpl_1","object":"chat.completion","model":"LongCat-Flash-Omni-2603",
			"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{
				"id":"call_1","type":"function","function":{"name":"apply_patch","arguments":"{\"input\":\"patch\"}"}
			}]},"finish_reason":"tool_calls"}]
		}`),
	})
	require.NoError(t, err)
	require.Len(t, response.Choices, 1)
	require.NotNil(t, response.Choices[0].Message)
	require.Len(t, response.Choices[0].Message.ToolCalls, 1)
	call := response.Choices[0].Message.ToolCalls[0]
	require.Equal(t, llm.ToolTypeResponsesCustomTool, call.Type)
	require.NotNil(t, call.ResponseCustomToolCall)
	require.Equal(t, "apply_patch", call.ResponseCustomToolCall.Name)
	require.Equal(t, "patch", call.ResponseCustomToolCall.Input)
}
