package copilot

import (
	"encoding/json"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestOutboundTransformer_ResponsesRequestCapabilitiesFollowResolvedModel(t *testing.T) {
	transformer := &OutboundTransformer{}
	require.False(t, transformer.ResponsesRequestCapabilities(&llm.Request{Model: "gpt-4o"}).NativeResponses)
	require.True(t, transformer.ResponsesRequestCapabilities(&llm.Request{Model: "gpt-5.6"}).NativeResponses)
	require.True(t, transformer.ResponsesRequestCapabilities(&llm.Request{
		Model: "gpt-5.6", RequestType: llm.RequestTypeCompact,
	}).NativeResponses)
}

func TestOutboundTransformer_TransformRequest_GPT4DowngradesResponsesLifecycleStandalone(t *testing.T) {
	transformer, err := NewOutboundTransformer(OutboundTransformerParams{
		TokenProvider: &mockTokenProvider{token: "ghu_testtoken123"},
	})
	require.NoError(t, err)

	request := responsesLifecycleRequestForPlainChatTest("gpt-4o")
	httpRequest, err := transformer.TransformRequest(t.Context(), request)
	require.NoError(t, err)
	assertPlainChatLifecycleDowngrade(t, httpRequest)
	require.Equal(t, llm.APIFormatOpenAIResponse, request.APIFormat)
	require.Len(t, request.Messages, 4)
	require.Len(t, request.Messages[1].ToolCalls, 2)
	require.Equal(t, llm.ToolTypeResponsesCustomTool, request.Messages[1].ToolCalls[0].Type)
	require.Equal(t, "custom_1", lo.FromPtr(request.Messages[2].ToolCallID))
	require.Len(t, request.Tools, 2)
	require.Equal(t, llm.ToolTypeResponsesCustomTool, request.Tools[0].Type)
	require.NotNil(t, request.ToolChoice)
	require.NotNil(t, request.ToolChoice.NamedToolChoice)
	require.Equal(t, "custom", request.ToolChoice.NamedToolChoice.Type)
}

func TestOutboundTransformer_TransformResponse_UnwrapsCompactResponsePreservingRequest(t *testing.T) {
	transformer, err := NewOutboundTransformer(OutboundTransformerParams{
		TokenProvider: &mockTokenProvider{token: "ghu_testtoken123"},
	})
	require.NoError(t, err)

	rawRequest, err := http.NewRequest(http.MethodPost, DefaultCopilotBaseURL+"/v1/responses/compact", nil)
	require.NoError(t, err)
	request := &httpclient.Request{
		RequestType: string(llm.RequestTypeCompact),
		APIFormat:   string(llm.APIFormatOpenAIResponseCompact),
	}
	response, err := transformer.TransformResponse(t.Context(), &httpclient.Response{
		StatusCode: http.StatusOK,
		Request:    request,
		RawRequest: rawRequest,
		Body: []byte(`{"response":{
			"id":"resp_compact_1",
			"object":"response.compaction",
			"created_at":1785720000,
			"model":"gpt-5.6",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"compact summary"}]}]
		}}`),
	})
	require.NoError(t, err)
	require.NotNil(t, response.Compact)
	require.Equal(t, llm.RequestTypeCompact, response.RequestType)
	require.Equal(t, "resp_compact_1", response.Compact.ID)
	require.Len(t, response.Compact.Output, 1)
	require.Equal(t, "compact summary", lo.FromPtr(response.Compact.Output[0].Content.Content))
}

func responsesLifecycleRequestForPlainChatTest(model string) *llm.Request {
	return &llm.Request{
		Model:     model,
		APIFormat: llm.APIFormatOpenAIResponse,
		Messages: []llm.Message{
			{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("run")}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{
				{
					ID: "custom_1", Type: llm.ToolTypeResponsesCustomTool,
					ResponseCustomToolCall: &llm.ResponseCustomToolCall{
						CallID: "custom_1", Name: "apply_patch", Input: "patch",
					},
				},
				{
					ID: "plain_1", Type: llm.ToolTypeFunction,
					Function: llm.FunctionCall{Name: "lookup", Arguments: `{}`},
				},
			}},
			{Role: "tool", ToolCallID: lo.ToPtr("custom_1"), Content: llm.MessageContent{Content: lo.ToPtr("patched")}},
			{Role: "tool", ToolCallID: lo.ToPtr("plain_1"), Content: llm.MessageContent{Content: lo.ToPtr("looked up")}},
		},
		Tools: []llm.Tool{
			{Type: llm.ToolTypeResponsesCustomTool, ResponseCustomTool: &llm.ResponseCustomTool{Name: "apply_patch"}},
			{Type: llm.ToolTypeFunction, Function: llm.Function{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}},
		},
		ToolChoice: &llm.ToolChoice{NamedToolChoice: &llm.NamedToolChoice{
			Type: "custom", Function: llm.ToolFunction{Name: "apply_patch"},
		}},
	}
}

func assertPlainChatLifecycleDowngrade(t *testing.T, request *httpclient.Request) {
	t.Helper()
	require.NotNil(t, request)
	var payload struct {
		Messages []struct {
			Role       string  `json:"role"`
			ToolCallID *string `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
		ToolChoice json.RawMessage `json:"tool_choice"`
	}
	require.NoError(t, json.Unmarshal(request.Body, &payload))
	require.Len(t, payload.Messages, 3)
	require.Len(t, payload.Messages[1].ToolCalls, 1)
	require.Equal(t, "plain_1", payload.Messages[1].ToolCalls[0].ID)
	require.Equal(t, llm.ToolTypeFunction, payload.Messages[1].ToolCalls[0].Type)
	require.Equal(t, "lookup", payload.Messages[1].ToolCalls[0].Function.Name)
	require.Equal(t, "plain_1", lo.FromPtr(payload.Messages[2].ToolCallID))
	require.Len(t, payload.Tools, 1)
	require.Equal(t, "lookup", payload.Tools[0].Function.Name)
	require.Empty(t, payload.ToolChoice)
	require.NotContains(t, request.TransformerMetadata, "openai_responses_chat_tool_mappings")
}
