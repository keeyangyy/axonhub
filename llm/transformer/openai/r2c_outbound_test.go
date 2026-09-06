package openai

import (
	"encoding/json"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestOutboundTransformer_ResponsesCapabilitiesExcludeCompactAPIFormat(t *testing.T) {
	transformer := &OutboundTransformer{}
	assert.True(t, transformer.ResponsesRequestCapabilities(&llm.Request{}).ChatToolLifecycle)
	assert.False(t, transformer.ResponsesRequestCapabilities(&llm.Request{
		APIFormat: llm.APIFormatOpenAIResponseCompact,
	}).ChatToolLifecycle)
	assert.False(t, transformer.ResponsesRequestCapabilities(&llm.Request{
		RequestType: llm.RequestTypeCompact,
	}).ChatToolLifecycle)
}

func TestOutboundTransformer_NativeChatBypassesResponsesToolAdapter(t *testing.T) {
	outbound, err := NewOutboundTransformer("https://chat.example.com", "test-key")
	assert.NoError(t, err)

	blank := " \n "
	request, err := outbound.TransformRequest(t.Context(), &llm.Request{
		Model:     "chat-model",
		APIFormat: llm.APIFormatOpenAIChatCompletion,
		Messages: []llm.Message{
			{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("before")}},
			{Role: "assistant", Content: llm.MessageContent{Content: &blank}},
			{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("after")}},
		},
		Tools: []llm.Tool{{
			Type: llm.ToolTypeFunction,
			Function: llm.Function{
				Name:       "lookup",
				Parameters: json.RawMessage(`{"properties":{"query":{"type":"string"}}}`),
			},
		}},
		TransformerMetadata: map[string]any{"existing": "kept"},
	})
	assert.NoError(t, err)

	var converted Request
	assert.NoError(t, json.Unmarshal(request.Body, &converted))
	assert.Len(t, converted.Messages, 3)
	assert.Equal(t, blank, lo.FromPtr(converted.Messages[1].Content.Content))
	assert.Len(t, converted.Tools, 1)
	assert.JSONEq(t, `{"properties":{"query":{"type":"string"}}}`, string(converted.Tools[0].Function.Parameters))
	assert.Equal(t, map[string]any{"existing": "kept"}, request.TransformerMetadata)
	_, hasMappings := request.TransformerMetadata[ResponsesChatToolMappingsMetadataKey]
	assert.False(t, hasMappings)
	_, hasWarnings := request.TransformerMetadata[responsesChatToolWarningsMetadataKey]
	assert.False(t, hasWarnings)
}

func TestOutboundTransformer_ResponsesRequestUsesToolAdapter(t *testing.T) {
	outbound, err := NewOutboundTransformer("https://chat.example.com", "test-key")
	assert.NoError(t, err)

	request, err := outbound.TransformRequest(t.Context(), &llm.Request{
		Model:     "chat-model",
		APIFormat: llm.APIFormatOpenAIResponse,
		Messages:  []llm.Message{{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("patch")}}},
		Tools: []llm.Tool{{
			Type:               llm.ToolTypeResponsesCustomTool,
			ResponseCustomTool: &llm.ResponseCustomTool{Name: "apply_patch"},
		}},
	})
	assert.NoError(t, err)

	var converted Request
	assert.NoError(t, json.Unmarshal(request.Body, &converted))
	assert.Len(t, converted.Tools, 1)
	assert.Equal(t, llm.ToolTypeFunction, converted.Tools[0].Type)
	assert.NotEmpty(t, responsesChatToolMappings(request))
}

func TestOutboundTransformer_TransformRequest_ReportsUnsupportedResponsesToolChoiceAsInvalidRequest(t *testing.T) {
	out, err := NewOutboundTransformer("https://api.openai.com/v1", "test-key")
	assert.NoError(t, err)

	_, err = out.TransformRequest(t.Context(), &llm.Request{
		Model:     "gpt-4o",
		APIFormat: llm.APIFormatOpenAIResponse,
		Messages: []llm.Message{{
			Role:    "user",
			Content: llm.MessageContent{Content: lo.ToPtr("use hosted search")},
		}},
		Tools: []llm.Tool{{
			Type: llm.ToolTypeResponsesToolSearch,
			ResponseToolSearch: &llm.ResponseToolSearch{
				Execution: "server",
			},
		}},
		ToolChoice: &llm.ToolChoice{NamedToolChoice: &llm.NamedToolChoice{
			Type: "tool_search",
			Function: llm.ToolFunction{
				Name: "tool_search",
			},
		}},
	})

	assert.ErrorIs(t, err, transformer.ErrInvalidRequest)
	assert.ErrorContains(t, err, "unsupported_tool_choice")
}
