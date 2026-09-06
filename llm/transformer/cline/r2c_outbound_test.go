package cline

import (
	"encoding/json"
	"github.com/looplj/axonhub/llm"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTransformRequest_DoesNotEnableUnrestorableResponsesToolAdapter(t *testing.T) {
	transformer := newTestTransformer(t)
	content := "run"
	customCallID := "custom_1"
	plainCallID := "plain_1"
	customResult := "patched"
	plainResult := "looked up"
	request := &llm.Request{
		Model:     "test-model",
		APIFormat: llm.APIFormatOpenAIResponse,
		Messages: []llm.Message{
			{Role: "user", Content: llm.MessageContent{Content: &content}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{
				{
					ID: customCallID, Type: llm.ToolTypeResponsesCustomTool,
					ResponseCustomToolCall: &llm.ResponseCustomToolCall{CallID: customCallID, Name: "apply_patch", Input: "patch"},
				},
				{ID: plainCallID, Type: llm.ToolTypeFunction, Function: llm.FunctionCall{Name: "lookup", Arguments: `{}`}},
			}},
			{Role: "tool", ToolCallID: &customCallID, Content: llm.MessageContent{Content: &customResult}},
			{Role: "tool", ToolCallID: &plainCallID, Content: llm.MessageContent{Content: &plainResult}},
		},
		Tools: []llm.Tool{
			{Type: llm.ToolTypeResponsesCustomTool, ResponseCustomTool: &llm.ResponseCustomTool{Name: "apply_patch"}},
			{Type: llm.ToolTypeFunction, Function: llm.Function{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}},
		},
		ToolChoice: &llm.ToolChoice{NamedToolChoice: &llm.NamedToolChoice{
			Type: "custom", Function: llm.ToolFunction{Name: "apply_patch"},
		}},
	}
	httpRequest, err := transformer.TransformRequest(t.Context(), request)
	require.NoError(t, err)

	var payload struct {
		Messages []struct {
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
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
		ToolChoice json.RawMessage `json:"tool_choice"`
	}
	require.NoError(t, json.Unmarshal(httpRequest.Body, &payload))
	require.Len(t, payload.Messages, 3)
	require.Len(t, payload.Messages[1].ToolCalls, 1)
	require.Equal(t, plainCallID, payload.Messages[1].ToolCalls[0].ID)
	require.Equal(t, llm.ToolTypeFunction, payload.Messages[1].ToolCalls[0].Type)
	require.Equal(t, "lookup", payload.Messages[1].ToolCalls[0].Function.Name)
	require.Equal(t, plainCallID, *payload.Messages[2].ToolCallID)
	require.Len(t, payload.Tools, 1)
	require.Equal(t, "lookup", payload.Tools[0].Function.Name)
	require.Empty(t, payload.ToolChoice)
	require.NotContains(t, httpRequest.TransformerMetadata, "openai_responses_chat_tool_mappings")
	require.Equal(t, llm.APIFormatOpenAIResponse, request.APIFormat)
	require.Len(t, request.Messages, 4)
	require.Len(t, request.Messages[1].ToolCalls, 2)
	require.Equal(t, llm.ToolTypeResponsesCustomTool, request.Messages[1].ToolCalls[0].Type)
	require.Equal(t, customCallID, *request.Messages[2].ToolCallID)
	require.Len(t, request.Tools, 2)
	require.NotNil(t, request.ToolChoice)
	require.NotNil(t, request.ToolChoice.NamedToolChoice)
}
