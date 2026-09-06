package responses

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

func TestInboundTransformer_PromotesAdditionalAndToolSearchTools(t *testing.T) {
	transformer := NewInboundTransformer()
	request, err := transformer.TransformRequest(context.Background(), &httpclient.Request{Body: []byte(`{
		"model":"gpt-5.5",
		"input":[
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"custom","name":"exec","description":"Run code"},
				{"type":"namespace","name":"collaboration","tools":[
					{"type":"function","name":"spawn_agent","parameters":{"type":"object","properties":{}}}
				]}
			]},
			{"type":"tool_search_call","execution":"client","call_id":"call_search","arguments":{"query":"agents"}},
			{"type":"tool_search_output","execution":"client","call_id":"call_search","tools":[
				{"type":"function","name":"send_message","parameters":{"type":"object","properties":{}}}
			]},
			{"role":"user","type":"message","content":[{"type":"input_text","text":"continue"}]}
		],
		"tools":[{"type":"tool_search","execution":"client","description":"Find tools","parameters":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}]
	}`)})
	require.NoError(t, err)
	require.Len(t, request.Tools, 4)
	require.Equal(t, llm.ToolTypeResponsesToolSearch, request.Tools[0].Type)
	require.Equal(t, llm.ToolTypeResponsesCustomTool, request.Tools[1].Type)
	require.Equal(t, "collaboration", request.Tools[2].Function.Namespace)
	require.Equal(t, "collaboration__spawn_agent", request.Tools[2].Function.Name)
	require.Equal(t, "additional_tools", request.Tools[2].ResponsesOrigin)
	require.Equal(t, "send_message", request.Tools[3].Function.Name)
	require.Equal(t, "tool_search_output", request.Tools[3].ResponsesOrigin)

	require.Len(t, request.Messages, 3)
	require.NotNil(t, request.Messages[0].ToolCalls[0].ResponseToolSearchCall)
	require.Equal(t, "call_search", lo.FromPtr(request.Messages[1].ToolCallID))
	require.JSONEq(t, `[{"type":"function","name":"send_message","parameters":{"type":"object","properties":{}}}]`, *request.Messages[1].Content.Content)
}

func TestInboundTransformer_PromotesNamespaceCustomTool(t *testing.T) {
	transformer := NewInboundTransformer()
	request, err := transformer.TransformRequest(context.Background(), &httpclient.Request{Body: []byte(`{
		"model":"gpt-5.5",
		"input":[{"type":"additional_tools","role":"developer","tools":[
			{"type":"namespace","name":"functions","tools":[
				{"type":"custom","name":"exec","description":"Run code","format":{"type":"grammar","syntax":"lark","definition":"start: SOURCE"}},
				{"type":"function","name":"wait","parameters":{"type":"object","properties":{}}}
			]}
		]}]
	}`)})
	require.NoError(t, err)
	require.Len(t, request.Tools, 2)

	require.Equal(t, llm.ToolTypeResponsesCustomTool, request.Tools[0].Type)
	require.NotNil(t, request.Tools[0].ResponseCustomTool)
	require.Equal(t, "exec", request.Tools[0].ResponseCustomTool.Name)
	require.Equal(t, "functions", request.Tools[0].ResponseCustomTool.Namespace)
	require.Equal(t, "Run code", request.Tools[0].ResponseCustomTool.Description)
	require.NotNil(t, request.Tools[0].ResponseCustomTool.Format)
	require.Equal(t, "grammar", request.Tools[0].ResponseCustomTool.Format.Type)
	require.Equal(t, "lark", request.Tools[0].ResponseCustomTool.Format.Syntax)
	require.Equal(t, "start: SOURCE", request.Tools[0].ResponseCustomTool.Format.Definition)

	require.Equal(t, "function", request.Tools[1].Type)
	require.Equal(t, "functions__wait", request.Tools[1].Function.Name)
	require.Equal(t, "functions", request.Tools[1].Function.Namespace)
}

func TestInboundTransformer_RejectsDuplicateTopLevelNamespaces(t *testing.T) {
	transformer := NewInboundTransformer()
	request, err := transformer.TransformRequest(context.Background(), &httpclient.Request{Body: []byte(`{
		"model":"gpt-5.5",
		"input":"use tools",
		"tools":[
			{"type":"namespace","name":"functions","tools":[{"type":"function","name":"one","parameters":{"type":"object"}}]},
			{"type":"function","name":"mid","parameters":{"type":"object"}},
			{"type":"namespace","name":"functions","tools":[{"type":"function","name":"two","parameters":{"type":"object"}}]},
			{"type":"future_server_tool","name":"future","future_option":{"mode":"keep"}}
		]
	}`)})
	require.Nil(t, request)
	require.ErrorContains(t, err, `duplicate_namespace: namespace "functions" appears in multiple tool declarations`)
}

func TestInboundTransformer_EncodesEmptyToolSearchOutputAsArray(t *testing.T) {
	transformer := NewInboundTransformer()
	request, err := transformer.TransformRequest(context.Background(), &httpclient.Request{Body: []byte(`{
		"model":"gpt-5.5",
		"input":[
			{"type":"tool_search_call","execution":"client","call_id":"call_search","arguments":{"query":"agents"}},
			{"type":"tool_search_output","execution":"client","call_id":"call_search"}
		]
	}`)})
	require.NoError(t, err)
	require.Len(t, request.Messages, 2)
	require.Equal(t, "[]", lo.FromPtr(request.Messages[1].Content.Content))
}

func TestInboundTransformer_PromotesFutureClientFunctionLikeTools(t *testing.T) {
	transformer := NewInboundTransformer()
	request, err := transformer.TransformRequest(context.Background(), &httpclient.Request{Body: []byte(`{
		"model":"gpt-5.5",
		"input":[{"type":"additional_tools","role":"developer","tools":[{
			"type":"future_client_tool","name":"later_lookup","description":"Lookup later",
			"execution":"client","parameters":{"type":"object","properties":{"id":{"type":"string"}}}
		}]}],
		"tools":[{
			"type":"future_client_tool","name":"lookup","description":"Lookup",
			"execution":"client","parameters":{"type":"object","properties":{"query":{"type":"string"}}}
		}]
	}`)})
	require.NoError(t, err)
	require.Len(t, request.Tools, 2)
	require.Equal(t, llm.ToolTypeFunction, request.Tools[0].Type)
	require.Equal(t, "lookup", request.Tools[0].Function.Name)
	require.Equal(t, "raw_tool", request.Tools[0].ResponsesOrigin)
	require.Equal(t, "future_client_tool", request.Tools[0].ResponsesSourceType)
	require.JSONEq(t, `{"type":"object","properties":{"query":{"type":"string"}}}`, string(request.Tools[0].Function.Parameters))
	require.Equal(t, llm.ToolTypeFunction, request.Tools[1].Type)
	require.Equal(t, "later_lookup", request.Tools[1].Function.Name)
	require.Equal(t, "additional_tools", request.Tools[1].ResponsesOrigin)
	require.Equal(t, "future_client_tool", request.Tools[1].ResponsesSourceType)
}

func TestInboundTransformer_PreservesAllowedToolsChoice(t *testing.T) {
	transformer := NewInboundTransformer()
	request, err := transformer.TransformRequest(context.Background(), &httpclient.Request{Body: []byte(`{
		"model":"gpt-5.5","input":"run","tools":[
			{"type":"function","name":"lookup","parameters":{"type":"object"}},
			{"type":"custom","name":"apply_patch"}
		],
		"tool_choice":{"type":"allowed_tools","mode":"auto","tools":[
			{"type":"function","name":"lookup"},
			{"type":"custom","name":"apply_patch"}
		]}
	}`)})
	require.NoError(t, err)
	require.NotNil(t, request.ToolChoice)
	require.True(t, request.ToolChoice.AllowedToolsSet)
	require.Equal(t, "auto", lo.FromPtr(request.ToolChoice.ToolChoice))
	require.Equal(t, []llm.ToolOption{
		{Type: "function", Name: "lookup"},
		{Type: "custom", Name: "apply_patch"},
	}, request.ToolChoice.AllowedTools)
}

func TestInboundTransformer_PreservesEmptyAllowedToolsChoice(t *testing.T) {
	transformer := NewInboundTransformer()
	request, err := transformer.TransformRequest(context.Background(), &httpclient.Request{Body: []byte(`{
		"model":"gpt-5.5","input":"run",
		"tool_choice":{"type":"allowed_tools","mode":"auto","tools":[]}
	}`)})
	require.NoError(t, err)
	require.NotNil(t, request.ToolChoice)
	require.True(t, request.ToolChoice.AllowedToolsSet)
	require.Empty(t, request.ToolChoice.AllowedTools)
}

func TestInboundTransformer_PreservesUnsupportedFutureToolForExplicitRejection(t *testing.T) {
	transformer := NewInboundTransformer()
	request, err := transformer.TransformRequest(context.Background(), &httpclient.Request{Body: []byte(`{
		"model":"gpt-5.5","input":"hello","tools":[
			{"type":"future_server_tool","name":"hosted","execution":"server"},
			{"type":"future_unknown_tool","name":"unknown","parameters":{"type":"object"}}
		]
	}`)})
	require.NoError(t, err)
	require.Len(t, request.Tools, 2)
	require.Equal(t, llm.ToolTypeResponsesOpaqueTool, request.Tools[0].Type)
	require.Equal(t, "future_server_tool", request.Tools[0].ResponseOpaqueTool.SourceType)
	require.Equal(t, llm.ToolTypeResponsesOpaqueTool, request.Tools[1].Type)
	require.Equal(t, "future_unknown_tool", request.Tools[1].ResponseOpaqueTool.SourceType)
}

func TestInboundTransformer_DoesNotInferUnknownNamespaceExecutionOwner(t *testing.T) {
	transformer := NewInboundTransformer()
	request, err := transformer.TransformRequest(context.Background(), &httpclient.Request{Body: []byte(`{
		"model":"gpt-5.5","input":"hello","tools":[{
			"type":"namespace","name":"hosted","tools":[
				{"type":"future_hosted_tool","name":"implicit","parameters":{"type":"object"}},
				{"type":"future_client_tool","name":"explicit","execution":"client","parameters":{"type":"object"}}
			]
		}]
	}`)})
	require.NoError(t, err)
	require.Len(t, request.Tools, 2)
	require.Equal(t, llm.ToolTypeResponsesOpaqueTool, request.Tools[0].Type)
	require.Equal(t, "future_hosted_tool", request.Tools[0].ResponseOpaqueTool.SourceType)
	require.Equal(t, llm.ToolTypeFunction, request.Tools[1].Type)
	require.Equal(t, "hosted", request.Tools[1].Function.Namespace)
	require.Equal(t, "hosted__explicit", request.Tools[1].Function.Name)
}

func TestInboundTransformer_EmitsResponsesSpecialToolCalls(t *testing.T) {
	transformer := NewInboundTransformer()
	result, err := transformer.TransformResponse(context.Background(), &llm.Response{
		ID: "resp_1", Model: "glm-5.2",
		Choices: []llm.Choice{{Message: &llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "call_custom", ResponseCustomToolCall: &llm.ResponseCustomToolCall{CallID: "call_custom", Name: "apply_patch", Input: "*** Begin Patch"}},
			{ID: "call_search", ResponseToolSearchCall: &llm.ResponseToolSearchCall{CallID: "call_search", Execution: "client", Arguments: `{"query":"agents"}`}},
		}}}},
	})
	require.NoError(t, err)
	var response Response
	require.NoError(t, json.Unmarshal(result.Body, &response))
	require.Len(t, response.Output, 2)
	require.Equal(t, "custom_tool_call", response.Output[0].Type)
	require.Equal(t, "*** Begin Patch", *response.Output[0].Input)
	require.Equal(t, "tool_search_call", response.Output[1].Type)
	require.Equal(t, "client", response.Output[1].Execution)
	require.JSONEq(t, `{"query":"agents"}`, response.Output[1].Arguments)
}

func TestInboundTransformer_TransformRequest_MergesRepeatedToolOutputs(t *testing.T) {
	trans := NewInboundTransformer()

	result, err := trans.TransformRequest(context.Background(), &httpclient.Request{
		Body: []byte(`{
			"model": "gpt-4o",
			"input": [
				{"role": "user", "content": "run exec"},
				{
					"type": "custom_tool_call",
					"call_id": "call_1612831776e44d5ca84206c0",
					"name": "exec",
					"input": "const r = await tools.exec_command({cmd: 'echo hi'}); text(r.output);"
				},
				{
					"type": "custom_tool_call_output",
					"call_id": "call_1612831776e44d5ca84206c0",
					"output": [{"type": "input_text", "text": "Script completed\nWall time 5.2 seconds\nOutput:\n"}]
				},
				{
					"type": "custom_tool_call_output",
					"call_id": "call_1612831776e44d5ca84206c0",
					"name": "exec",
					"output": "notify 工具测试成功"
				}
			]
		}`),
	})

	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, result.Messages, 3)
	require.Equal(t, "assistant", result.Messages[1].Role)
	require.Len(t, result.Messages[1].ToolCalls, 1)
	require.Equal(t, "tool", result.Messages[2].Role)
	require.Equal(t, "call_1612831776e44d5ca84206c0", lo.FromPtr(result.Messages[2].ToolCallID))

	merged := flattenToolContent(t, result.Messages[2].Content)
	require.Contains(t, merged, "Script completed")
	require.Contains(t, merged, "notify 工具测试成功")
}

func TestInboundTransformer_PreservesNamespaceCustomAndFunctionToolCalls(t *testing.T) {
	transformer := NewInboundTransformer()
	request, err := transformer.TransformRequest(context.Background(), &httpclient.Request{Body: []byte(`{
		"model": "gpt-5.5",
		"input": [
			{"type": "message", "role": "user", "content": "run tools"},
			{"type": "function_call", "call_id": "call_lookup", "namespace": "functions", "name": "lookup", "arguments": "{}"},
			{"type": "custom_tool_call", "call_id": "call_exec", "namespace": "functions", "name": "exec", "input": "ls"},
			{"type": "message", "role": "user", "content": "done"}
		]
	}`)})
	require.NoError(t, err)
	require.Len(t, request.Messages[1].ToolCalls, 2)
	require.Equal(t, "functions", request.Messages[1].ToolCalls[0].Function.Namespace)
	require.Equal(t, "functions", request.Messages[1].ToolCalls[1].ResponseCustomToolCall.Namespace)

	outbound, err := NewOutboundTransformer("https://api.openai.com", "test-api-key")
	require.NoError(t, err)
	httpRequest, err := outbound.TransformRequest(context.Background(), request)
	require.NoError(t, err)

	var payload struct {
		Input []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"input"`
	}
	require.NoError(t, json.Unmarshal(httpRequest.Body, &payload))
	calls := make([]struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}, 0, 2)
	for _, item := range payload.Input {
		if item.Type == "function_call" || item.Type == "custom_tool_call" {
			calls = append(calls, item)
		}
	}
	require.Len(t, calls, 2)
	require.Equal(t, "function_call", calls[0].Type)
	require.Equal(t, "lookup", calls[0].Name)
	require.Equal(t, "functions", calls[0].Namespace)
	require.Equal(t, "custom_tool_call", calls[1].Type)
	require.Equal(t, "exec", calls[1].Name)
	require.Equal(t, "functions", calls[1].Namespace)
}

func TestInboundTransformer_TransformRequest_MergesRepeatedFunctionOutputs(t *testing.T) {
	trans := NewInboundTransformer()

	result, err := trans.TransformRequest(context.Background(), &httpclient.Request{
		Body: []byte(`{
			"model": "gpt-4o",
			"input": [
				{"role": "user", "content": "run fn"},
				{"type": "function_call", "call_id": "call_x", "name": "fn", "arguments": "{}"},
				{"type": "function_call_output", "call_id": "call_x", "output": "first"},
				{"type": "function_call_output", "call_id": "call_x", "output": "second"}
			]
		}`),
	})

	require.NoError(t, err)
	require.Len(t, result.Messages, 3)
	require.Equal(t, "tool", result.Messages[2].Role)
	require.Equal(t, "call_x", lo.FromPtr(result.Messages[2].ToolCallID))
	merged := flattenToolContent(t, result.Messages[2].Content)
	require.Contains(t, merged, "first")
	require.Contains(t, merged, "second")
}

func flattenToolContent(t *testing.T, c llm.MessageContent) string {
	t.Helper()
	if len(c.MultipleContent) == 0 && c.Content != nil {
		return *c.Content
	}
	var b strings.Builder
	for _, p := range c.MultipleContent {
		if p.Type != "text" {
			t.Fatalf("flattenToolContent: unexpected non-text part type %q", p.Type)
		}
		if p.Text != nil {
			b.WriteString(*p.Text)
		}
	}
	return b.String()
}

func TestConvertToResponsesAPIResponse_AbnormalFinishKeepsToolCallInProgress(t *testing.T) {
	tests := []struct {
		finishReason string
		status       string
	}{
		{finishReason: "length", status: "incomplete"},
		{finishReason: "content_filter", status: "incomplete"},
		{finishReason: "error", status: "failed"},
		{finishReason: "cancelled", status: "cancelled"},
		{finishReason: "canceled", status: "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.finishReason, func(t *testing.T) {
			response := convertToResponsesAPIResponse(&llm.Response{
				ID: "chatcmpl_abnormal", Model: "glm", Choices: []llm.Choice{{
					FinishReason: lo.ToPtr(tt.finishReason),
					Message: &llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{
						ID: "call_partial", Type: llm.ToolTypeFunction,
						Function: llm.FunctionCall{Name: "lookup", Arguments: `{"query":"partial`},
					}}},
				}},
			})

			require.Equal(t, tt.status, lo.FromPtr(response.Status))
			require.Len(t, response.Output, 1)
			require.Equal(t, "function_call", response.Output[0].Type)
			require.Equal(t, "in_progress", lo.FromPtr(response.Output[0].Status))
		})
	}
}

func TestConvertContentItemToPart_EncryptedContent(t *testing.T) {
	tests := []struct {
		name      string
		item      *Item
		ownerType string
		validate  func(t *testing.T, result *llm.MessageContentPart, err error)
	}{
		{
			name:      "agent_message owner converts encrypted_content to text",
			item:      &Item{ID: "enc_1", Type: "encrypted_content", EncryptedContent: lo.ToPtr("dispatched task")},
			ownerType: "agent_message",
			validate: func(t *testing.T, result *llm.MessageContentPart, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, "text", result.Type)
				require.Equal(t, "dispatched task", *result.Text)
			},
		},
		{
			name:      "agent_message owner drops empty encrypted_content",
			item:      &Item{ID: "enc_2", Type: "encrypted_content", EncryptedContent: lo.ToPtr("")},
			ownerType: "agent_message",
			validate: func(t *testing.T, result *llm.MessageContentPart, err error) {
				require.NoError(t, err)
				require.Nil(t, result)
			},
		},
		{
			name:      "message owner does not surface encrypted_content as text",
			item:      &Item{ID: "enc_3", Type: "encrypted_content", EncryptedContent: lo.ToPtr("opaque")},
			ownerType: "message",
			validate: func(t *testing.T, result *llm.MessageContentPart, err error) {
				require.NoError(t, err)
				require.Nil(t, result)
			},
		},
		{
			name:      "function_call_output owner does not surface encrypted_content as text",
			item:      &Item{ID: "enc_4", Type: "encrypted_content", EncryptedContent: lo.ToPtr("opaque")},
			ownerType: "function_call_output",
			validate: func(t *testing.T, result *llm.MessageContentPart, err error) {
				require.NoError(t, err)
				require.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertContentItemToPart(tt.item, tt.ownerType)
			tt.validate(t, result, err)
		})
	}
}

func TestConvertItemToMessage_AgentMessage(t *testing.T) {
	tests := []struct {
		name     string
		item     *Item
		validate func(t *testing.T, result *llm.Message, err error)
	}{
		{
			name: "agent_message with input_text and encrypted_content parts",
			item: &Item{
				ID:   "amsg_1",
				Type: "agent_message",
				Content: &Input{Items: []Item{
					{Type: "input_text", Text: lo.ToPtr("Message Type: NEW_TASK\nTask name: /root/say_hello\nSender: /root\nPayload:\n")},
					{Type: "encrypted_content", EncryptedContent: lo.ToPtr("请用中文向用户打个招呼,做一个简短的自我介绍(说明你是一个子 agent),然后就结束,不要做其他工作。")},
				}},
			},
			validate: func(t *testing.T, result *llm.Message, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, "user", result.Role)
				require.Len(t, result.Content.MultipleContent, 2)
				require.Equal(t, "text", result.Content.MultipleContent[0].Type)
				require.Contains(t, *result.Content.MultipleContent[0].Text, "NEW_TASK")
				require.Equal(t, "text", result.Content.MultipleContent[1].Type)
				require.Contains(t, *result.Content.MultipleContent[1].Text, "打个招呼")
			},
		},
		{
			name: "agent_message with only encrypted_content part",
			item: &Item{
				ID:   "amsg_2",
				Type: "agent_message",
				Content: &Input{Items: []Item{
					{Type: "encrypted_content", EncryptedContent: lo.ToPtr("仅一句问候语作为最终答案")},
				}},
			},
			validate: func(t *testing.T, result *llm.Message, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, "user", result.Role)
				require.NotNil(t, result.Content.Content)
				require.Contains(t, *result.Content.Content, "问候语")
			},
		},
		{
			name: "agent_message with empty encrypted_content part is dropped",
			item: &Item{
				ID:   "amsg_3",
				Type: "agent_message",
				Content: &Input{Items: []Item{
					{Type: "input_text", Text: lo.ToPtr("header")},
					{Type: "encrypted_content", EncryptedContent: lo.ToPtr("")},
				}},
			},
			validate: func(t *testing.T, result *llm.Message, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, "user", result.Role)
				require.NotNil(t, result.Content.Content)
				require.Contains(t, *result.Content.Content, "header")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertItemToMessage(tt.item)
			tt.validate(t, result, err)
		})
	}
}

func TestInboundTransformer_TransformRequest_WithAgentMessage(t *testing.T) {
	trans := NewInboundTransformer()

	httpReq := &httpclient.Request{
		Body: []byte(`{
			"model": "gpt-4o",
			"input": [
				{
					"type": "message",
					"role": "user",
					"content": "请帮我起一个子agent打招呼"
				},
				{
					"type": "agent_message",
					"id": "amsg_1",
					"author": "/root",
					"recipient": "/root/say_hello",
					"content": [
						{"type": "input_text", "text": "Message Type: NEW_TASK\nTask name: /root/say_hello\nSender: /root\nPayload:\n"},
						{"type": "encrypted_content", "encrypted_content": "请用中文向用户打个招呼,做一个简短的自我介绍(说明你是一个子 agent),然后就结束,不要做其他工作。"}
					]
				}
			]
		}`),
	}

	result, err := trans.TransformRequest(context.Background(), httpReq)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Messages, 2)

	require.Equal(t, "user", result.Messages[0].Role)
	require.Equal(t, "请帮我起一个子agent打招呼", *result.Messages[0].Content.Content)

	require.Equal(t, "user", result.Messages[1].Role)
	require.Len(t, result.Messages[1].Content.MultipleContent, 2)
	require.Equal(t, "text", result.Messages[1].Content.MultipleContent[0].Type)
	require.Contains(t, *result.Messages[1].Content.MultipleContent[0].Text, "NEW_TASK")
	require.Equal(t, "text", result.Messages[1].Content.MultipleContent[1].Type)
	require.Contains(t, *result.Messages[1].Content.MultipleContent[1].Text, "打个招呼")
}

func TestConvertToolChoiceToLLMPrimitiveMatrix(t *testing.T) {
	t.Run("named selectors retain primitive identity", func(t *testing.T) {
		tests := []struct {
			toolType string
			name     string
		}{
			{toolType: "function", name: "lookup"},
			{toolType: "custom", name: "apply_patch"},
			{toolType: "namespace", name: "workspace"},
			{toolType: "tool_search", name: "discover"},
			{toolType: "future_client_tool", name: "later"},
			{toolType: "future_server_tool", name: "hosted"},
		}
		for _, tt := range tests {
			t.Run(tt.toolType, func(t *testing.T) {
				result := convertToolChoiceToLLM(&ToolChoice{
					Type: lo.ToPtr(tt.toolType),
					Name: lo.ToPtr(tt.name),
				})
				require.NotNil(t, result)
				require.Nil(t, result.ToolChoice)
				require.NotNil(t, result.NamedToolChoice)
				require.Equal(t, tt.toolType, result.NamedToolChoice.Type)
				require.Equal(t, tt.name, result.NamedToolChoice.Function.Name)
				require.False(t, result.AllowedToolsSet)
				require.Nil(t, result.AllowedTools)
			})
		}
	})

	t.Run("allowed selectors retain same name across primitive types", func(t *testing.T) {
		for _, mode := range []string{"auto", "required"} {
			t.Run(mode, func(t *testing.T) {
				result := convertToolChoiceToLLM(&ToolChoice{
					Type: lo.ToPtr("allowed_tools"),
					Mode: lo.ToPtr(mode),
					Tools: []ToolOption{
						{Type: "function", Name: "same"},
						{Type: "custom", Name: "same"},
						{Type: "namespace", Name: "workspace"},
						{Type: "tool_search", Name: "discover"},
						{Type: "future_client_tool", Name: "later"},
						{Type: "future_server_tool", Name: "hosted"},
					},
				})
				require.NotNil(t, result)
				require.Equal(t, mode, lo.FromPtr(result.ToolChoice))
				require.Nil(t, result.NamedToolChoice)
				require.True(t, result.AllowedToolsSet)
				require.Equal(t, []llm.ToolOption{
					{Type: "function", Name: "same"},
					{Type: "custom", Name: "same"},
					{Type: "namespace", Name: "workspace"},
					{Type: "tool_search", Name: "discover"},
					{Type: "future_client_tool", Name: "later"},
					{Type: "future_server_tool", Name: "hosted"},
				}, result.AllowedTools)
			})
		}
	})

	t.Run("type only selectors retain type without name", func(t *testing.T) {
		for _, toolType := range []string{"web_search", "future_server_tool", "mcp"} {
			t.Run(toolType, func(t *testing.T) {
				result := convertToolChoiceToLLM(&ToolChoice{Type: lo.ToPtr(toolType)})
				require.NotNil(t, result)
				require.Nil(t, result.ToolChoice)
				require.NotNil(t, result.NamedToolChoice)
				require.Equal(t, toolType, result.NamedToolChoice.Type)
				require.Empty(t, result.NamedToolChoice.Function.Name)
				require.False(t, result.AllowedToolsSet)
				require.Nil(t, result.AllowedTools)
			})
		}
	})
}

func TestConvertInputToMessages_GroupsConsecutiveMixedToolCalls(t *testing.T) {
	input := &Input{Items: []Item{
		{Type: "function_call", CallID: "function:1", Name: "lookup", Arguments: `{"id":"42"}`},
		{Type: "custom_tool_call", CallID: "custom:2", Name: "apply_patch", Input: lo.ToPtr("patch")},
		{Type: "tool_search_call", CallID: "search:3", Execution: "client", Arguments: `{"query":"agents"}`},
	}}

	messages, err := convertInputToMessages(input)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "assistant", messages[0].Role)
	require.Len(t, messages[0].ToolCalls, 3)
	require.Equal(t, []string{"function:1", "custom:2", "search:3"}, []string{
		messages[0].ToolCalls[0].ID,
		messages[0].ToolCalls[1].ID,
		messages[0].ToolCalls[2].ID,
	})
	require.Equal(t, []int{0, 1, 2}, []int{
		messages[0].ToolCalls[0].Index,
		messages[0].ToolCalls[1].Index,
		messages[0].ToolCalls[2].Index,
	})
	require.Equal(t, "lookup", messages[0].ToolCalls[0].Function.Name)
	require.NotNil(t, messages[0].ToolCalls[1].ResponseCustomToolCall)
	require.Equal(t, "apply_patch", messages[0].ToolCalls[1].ResponseCustomToolCall.Name)
	require.NotNil(t, messages[0].ToolCalls[2].ResponseToolSearchCall)
	require.Equal(t, "client", messages[0].ToolCalls[2].ResponseToolSearchCall.Execution)
}

func TestConvertInputToMessages_StopsToolCallGroupsAtBoundaries(t *testing.T) {
	functionCall := func(callID string) Item {
		return Item{Type: "function_call", CallID: callID, Name: "lookup", Arguments: `{}`}
	}
	customCall := func(callID string) Item {
		return Item{Type: "custom_tool_call", CallID: callID, Name: "apply_patch", Input: lo.ToPtr("patch")}
	}
	toolSearchCall := func(callID string) Item {
		return Item{Type: "tool_search_call", CallID: callID, Execution: "client", Arguments: `{}`}
	}

	tests := []struct {
		name      string
		items     []Item
		wantRoles []string
	}{
		{
			name: "function output",
			items: []Item{
				functionCall("call:1"),
				{Type: "function_call_output", CallID: "call:1", Output: &Input{Text: lo.ToPtr("result")}},
				functionCall("call:2"),
			},
			wantRoles: []string{"assistant", "tool", "assistant"},
		},
		{
			name: "custom output",
			items: []Item{
				customCall("call:1"),
				{Type: "custom_tool_call_output", CallID: "call:1", Output: &Input{Text: lo.ToPtr("result")}},
				functionCall("call:2"),
			},
			wantRoles: []string{"assistant", "tool", "assistant"},
		},
		{
			name: "tool search output",
			items: []Item{
				toolSearchCall("call:1"),
				{Type: "tool_search_output", CallID: "call:1", Tools: []Tool{}},
				functionCall("call:2"),
			},
			wantRoles: []string{"assistant", "tool", "assistant"},
		},
		{
			name: "user message",
			items: []Item{
				functionCall("call:1"),
				{Type: "message", Role: "user", Content: &Input{Text: lo.ToPtr("next turn")}},
				customCall("call:2"),
			},
			wantRoles: []string{"assistant", "user", "assistant"},
		},
		{
			name: "reasoning",
			items: []Item{
				functionCall("call:1"),
				{Type: "reasoning", Summary: []ReasoningSummary{{Type: "summary_text", Text: "next turn"}}},
				customCall("call:2"),
			},
			wantRoles: []string{"assistant", "assistant"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := convertInputToMessages(&Input{Items: tt.items})
			require.NoError(t, err)
			require.Len(t, messages, len(tt.wantRoles))
			roles := make([]string, len(messages))
			for i := range messages {
				roles[i] = messages[i].Role
			}
			require.Equal(t, tt.wantRoles, roles)
			require.Equal(t, "call:1", messages[0].ToolCalls[0].ID)
			require.Equal(t, "call:2", messages[len(messages)-1].ToolCalls[0].ID)
		})
	}
}

func TestInboundTransformer_TransformRequest_MergesInterleavedToolOutputs(t *testing.T) {
	trans := NewInboundTransformer()

	result, err := trans.TransformRequest(context.Background(), &httpclient.Request{
		Body: []byte(`{
			"model": "gpt-4o",
			"input": [
				{"role": "user", "content": "run both"},
				{"type": "function_call", "call_id": "call_a", "name": "fn", "arguments": "{}"},
				{"type": "function_call", "call_id": "call_b", "name": "fn", "arguments": "{}"},
				{"type": "function_call_output", "call_id": "call_a", "output": "a1"},
				{"type": "function_call_output", "call_id": "call_b", "output": "b1"},
				{"type": "function_call_output", "call_id": "call_a", "output": "a2"},
				{"role": "user", "content": "next"}
			]
		}`),
	})

	require.NoError(t, err)

	require.Len(t, result.Messages, 5)
	require.Equal(t, "assistant", result.Messages[1].Role)
	require.Len(t, result.Messages[1].ToolCalls, 2)

	seen := make(map[string]int)
	for _, message := range result.Messages {
		if message.Role == "tool" {
			seen[lo.FromPtr(message.ToolCallID)]++
		}
	}
	require.Equal(t, map[string]int{"call_a": 1, "call_b": 1}, seen)

	require.Equal(t, "tool", result.Messages[2].Role)
	require.Equal(t, "call_a", lo.FromPtr(result.Messages[2].ToolCallID))
	require.Equal(t, "a1a2", flattenToolContent(t, result.Messages[2].Content))
	require.Equal(t, "tool", result.Messages[3].Role)
	require.Equal(t, "call_b", lo.FromPtr(result.Messages[3].ToolCallID))
	require.Equal(t, "b1", flattenToolContent(t, result.Messages[3].Content))
}
