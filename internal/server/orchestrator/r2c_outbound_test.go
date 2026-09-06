package orchestrator

import (
	"context"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
	anthropictransformer "github.com/looplj/axonhub/llm/transformer/anthropic"
	bailiantransformer "github.com/looplj/axonhub/llm/transformer/bailian"
	longcattransformer "github.com/looplj/axonhub/llm/transformer/longcat"
	modelscopetransformer "github.com/looplj/axonhub/llm/transformer/modelscope"
	moonshottransformer "github.com/looplj/axonhub/llm/transformer/moonshot"
	openaitransformer "github.com/looplj/axonhub/llm/transformer/openai"
	openairesponses "github.com/looplj/axonhub/llm/transformer/openai/responses"
	xaitransformer "github.com/looplj/axonhub/llm/transformer/xai"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"net/http"
	"testing"
)

func TestResponsesRequestCapabilities_UsesExplicitProviderCapability(t *testing.T) {
	genericOpenAI, err := openaitransformer.NewOutboundTransformer("https://chat.example.com", "test-key")
	require.NoError(t, err)
	anthropic, err := anthropictransformer.NewOutboundTransformer("https://anthropic.example.com", "test-key")
	require.NoError(t, err)
	moonshot, err := moonshottransformer.NewOutboundTransformer("https://moonshot.example.com", "test-key")
	require.NoError(t, err)
	bailian, err := bailiantransformer.NewOutboundTransformer("https://bailian.example.com", "test-key")
	require.NoError(t, err)
	longcat, err := longcattransformer.NewOutboundTransformer("https://longcat.example.com", "test-key")
	require.NoError(t, err)
	modelscope, err := modelscopetransformer.NewOutboundTransformer("https://modelscope.example.com", "test-key")
	require.NoError(t, err)
	xai, err := xaitransformer.NewOutboundTransformer("https://xai.example.com", "test-key")
	require.NoError(t, err)
	nativeResponses, err := openairesponses.NewOutboundTransformer("https://responses.example.com", "test-key")
	require.NoError(t, err)

	require.True(t, transformer.ResponsesRequestCapabilitiesOf(genericOpenAI, &llm.Request{}).ChatToolLifecycle)
	require.True(t, transformer.ResponsesRequestCapabilitiesOf(nativeResponses, &llm.Request{}).NativeResponses)
	require.False(t, transformer.ResponsesRequestCapabilitiesOf(&mockTransformer{
		apiFormat: llm.APIFormatOpenAIResponse,
		responsesCapabilities: func(*llm.Request) transformer.ResponsesRequestCapabilities {
			return transformer.ResponsesRequestCapabilities{ChatToolLifecycle: true}
		},
	}, &llm.Request{}).NativeResponses)
	require.False(t, transformer.ResponsesRequestCapabilitiesOf(&mockTransformer{apiFormat: llm.APIFormatOpenAIChatCompletion}, &llm.Request{}).ChatToolLifecycle)

	require.False(t, transformer.ResponsesRequestCapabilitiesOf(anthropic, &llm.Request{}).ChatToolLifecycle)
	for _, outbound := range []transformer.Outbound{bailian, longcat, modelscope, xai, moonshot} {
		require.True(t, transformer.ResponsesRequestCapabilitiesOf(outbound, &llm.Request{}).ChatToolLifecycle)
	}
}

// newResponsesChatToolFilterRequest builds a fresh Responses-format request
// fixture so each sub-test mutates its own copy instead of sharing state.
func newResponsesChatToolFilterRequest() *llm.Request {
	return &llm.Request{
		APIFormat: llm.APIFormatOpenAIResponse,
		Tools: []llm.Tool{
			{Type: llm.ToolTypeFunction, Function: llm.Function{Name: "get_weather"}},
			{Type: llm.ToolTypeResponsesCustomTool, ResponseCustomTool: &llm.ResponseCustomTool{Name: "apply_patch"}},
			{Type: llm.ToolTypeResponsesToolSearch, ResponseToolSearch: &llm.ResponseToolSearch{Execution: "client"}},
			{Type: llm.ToolTypeFunction, Function: llm.Function{Name: "collaboration__spawn_agent", Namespace: "collaboration"}},
			{Type: llm.ToolTypeFunction, Function: llm.Function{Name: "future_lookup"}, ResponsesSourceType: "future_client_tool"},
		},
		ToolChoice: &llm.ToolChoice{
			ToolChoice: lo.ToPtr("auto"), AllowedToolsSet: true,
			AllowedTools: []llm.ToolOption{
				{Type: llm.ToolTypeFunction, Name: "get_weather"},
				{Type: "custom", Name: "apply_patch"},
			},
		},
		Messages: []llm.Message{
			{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{
					{
						ID:   "call_custom_1",
						Type: llm.ToolTypeResponsesCustomTool,
						ResponseCustomToolCall: &llm.ResponseCustomToolCall{
							CallID: "call_custom_1",
							Name:   "apply_patch",
							Input:  "*** Begin Patch\n*** End Patch\n",
						},
					},
					{
						ID:   "call_function_1",
						Type: llm.ToolTypeFunction,
						Function: llm.FunctionCall{
							Name:      "get_weather",
							Arguments: "{}",
						},
					},
					{
						ID:   "call_search_1",
						Type: llm.ToolTypeResponsesToolSearch,
						ResponseToolSearchCall: &llm.ResponseToolSearchCall{
							CallID:    "call_search_1",
							Execution: "client",
							Arguments: `{"query":"agents"}`,
						},
					},
					{
						ID:   "call_namespace_1",
						Type: llm.ToolTypeFunction,
						Function: llm.FunctionCall{
							Name:      "spawn_agent",
							Namespace: "collaboration",
							Arguments: `{}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				ToolCallID: lo.ToPtr("call_custom_1"),
				Content:    llm.MessageContent{Content: lo.ToPtr("custom")},
			},
			{
				Role:       "tool",
				ToolCallID: lo.ToPtr("call_function_1"),
				Content:    llm.MessageContent{Content: lo.ToPtr("function")},
			},
			{
				Role:       "tool",
				ToolCallID: lo.ToPtr("call_search_1"),
				Content:    llm.MessageContent{Content: lo.ToPtr("search")},
			},
			{
				Role:       "tool",
				ToolCallID: lo.ToPtr("call_namespace_1"),
				Content:    llm.MessageContent{Content: lo.ToPtr("spawned")},
			},
		},
	}
}

func newResponsesInvalidToolArgumentRequest() *llm.Request {
	return &llm.Request{
		APIFormat: llm.APIFormatOpenAIResponse,
		Messages: []llm.Message{{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID: "call_truncated", Type: llm.ToolTypeFunction,
				Function: llm.FunctionCall{Name: "lookup", Arguments: `{"query":"open`},
			}},
		}},
	}
}

func nativeResponsesCapabilities(*llm.Request) transformer.ResponsesRequestCapabilities {
	return transformer.ResponsesRequestCapabilities{NativeResponses: true}
}

func TestFilterResponsesChatToolMessagesForOutbound_ToolArgumentSanitization(t *testing.T) {
	t.Run("native responses preserves invalid arguments", func(t *testing.T) {
		request := newResponsesInvalidToolArgumentRequest()
		got, err := filterResponsesChatToolMessagesForOutbound(request, &mockTransformer{
			apiFormat: llm.APIFormatOpenAIResponse, responsesCapabilities: nativeResponsesCapabilities,
		})
		require.NoError(t, err)
		require.Same(t, request, got)
		require.Equal(t, `{"query":"open`, request.Messages[0].ToolCalls[0].Function.Arguments)
	})

	t.Run("chat lifecycle repairs invalid arguments", func(t *testing.T) {
		request := newResponsesInvalidToolArgumentRequest()
		got, err := filterResponsesChatToolMessagesForOutbound(request, &mockTransformer{
			apiFormat: llm.APIFormatOpenAIChatCompletion, responsesChatTools: true,
		})
		require.NoError(t, err)
		require.NotSame(t, request, got)
		require.Equal(t, `{"query":"open`, request.Messages[0].ToolCalls[0].Function.Arguments)
		require.JSONEq(t, `{}`, got.Messages[0].ToolCalls[0].Function.Arguments)
	})
}

func TestFilterResponsesChatToolMessagesForOutbound_PreviousResponseID(t *testing.T) {
	t.Run("native responses preserves previous response id", func(t *testing.T) {
		request := &llm.Request{
			APIFormat:          llm.APIFormatOpenAIResponse,
			PreviousResponseID: lo.ToPtr("resp_prev_123"),
		}
		got, err := filterResponsesChatToolMessagesForOutbound(
			request,
			&mockTransformer{apiFormat: llm.APIFormatOpenAIResponse, responsesCapabilities: nativeResponsesCapabilities},
		)
		require.NoError(t, err)
		require.Same(t, request, got)
		require.Same(t, request.PreviousResponseID, got.PreviousResponseID)
	})

	t.Run("non-native outbound rejects previous response id", func(t *testing.T) {
		request := &llm.Request{
			APIFormat:          llm.APIFormatOpenAIResponse,
			PreviousResponseID: lo.ToPtr("resp_prev_123"),
		}
		got, err := filterResponsesChatToolMessagesForOutbound(
			request,
			&mockTransformer{apiFormat: llm.APIFormatOpenAIChatCompletion, responsesChatTools: true},
		)
		require.Nil(t, got)
		require.ErrorIs(t, err, transformer.ErrInvalidRequest)
		require.EqualError(t, err, "invalid request: previous_response_id requires a native Responses outbound because fallback channels cannot preserve Responses history")
	})

	t.Run("non-native outbound without previous response id is accepted", func(t *testing.T) {
		request := &llm.Request{APIFormat: llm.APIFormatOpenAIResponse}
		got, err := filterResponsesChatToolMessagesForOutbound(
			request,
			&mockTransformer{apiFormat: llm.APIFormatOpenAIChatCompletion, responsesChatTools: true},
		)
		require.NoError(t, err)
		require.Same(t, request, got)
	})
}

func TestPersistentOutboundTransformer_TransformRequest_PreviousResponseID(t *testing.T) {
	newProcessor := func(outbound transformer.Outbound) (*PersistentOutboundTransformer, *llm.Request) {
		channel := &biz.Channel{
			Channel: &ent.Channel{
				ID:              1,
				Name:            "test-channel",
				SupportedModels: []string{"gpt-4"},
			},
			Outbound: outbound,
		}
		return &PersistentOutboundTransformer{
				wrapped: outbound,
				state: &PersistenceState{
					ChannelModelsCandidates: []*ChannelModelsCandidate{{
						Channel: channel,
						Models:  []biz.ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4"}},
					}},
					CurrentCandidateIndex: 0,
				},
			}, &llm.Request{
				APIFormat: llm.APIFormatOpenAIResponse,
				Model:     "gpt-4",
			}
	}

	t.Run("native responses passes the request through", func(t *testing.T) {
		outbound := &mockTransformer{
			apiFormat:             llm.APIFormatOpenAIResponse,
			requestAPIFormat:      llm.APIFormatOpenAIResponse,
			responsesCapabilities: nativeResponsesCapabilities,
		}
		processor, request := newProcessor(outbound)
		request.PreviousResponseID = lo.ToPtr("resp_prev_123")

		httpRequest, err := processor.TransformRequest(context.Background(), request)
		require.NoError(t, err)
		require.NotNil(t, httpRequest)
		require.Same(t, request, outbound.capturedRequest)
		require.Same(t, request.PreviousResponseID, outbound.capturedRequest.PreviousResponseID)
	})

	t.Run("chat outbound rejects before provider conversion", func(t *testing.T) {
		outbound := &mockTransformer{apiFormat: llm.APIFormatOpenAIChatCompletion, responsesChatTools: true}
		processor, request := newProcessor(outbound)
		request.PreviousResponseID = lo.ToPtr("resp_prev_123")

		httpRequest, err := processor.TransformRequest(context.Background(), request)
		require.Nil(t, httpRequest)
		require.ErrorIs(t, err, transformer.ErrInvalidRequest)
		require.Nil(t, outbound.capturedRequest)

		rawErr := openairesponses.NewInboundTransformer().TransformError(context.Background(), err)
		require.Equal(t, http.StatusBadRequest, rawErr.StatusCode)
		require.Equal(t, "invalid_request_error", gjson.GetBytes(rawErr.Body, "error.type").String())
		require.Equal(
			t,
			"invalid request: previous_response_id requires a native Responses outbound because fallback channels cannot preserve Responses history",
			gjson.GetBytes(rawErr.Body, "error.message").String(),
		)
	})

	t.Run("chat outbound accepts when previous response id is absent", func(t *testing.T) {
		outbound := &mockTransformer{apiFormat: llm.APIFormatOpenAIChatCompletion, responsesChatTools: true}
		processor, request := newProcessor(outbound)

		httpRequest, err := processor.TransformRequest(context.Background(), request)
		require.NoError(t, err)
		require.NotNil(t, httpRequest)
		require.Same(t, request, outbound.capturedRequest)
		require.Nil(t, outbound.capturedRequest.PreviousResponseID)
	})
}

func TestFilterResponsesChatToolMessagesForOutbound(t *testing.T) {
	t.Run("preserves when outbound Chat adapter supports custom lifecycle", func(t *testing.T) {
		request := newResponsesChatToolFilterRequest()
		outbound := &mockTransformer{apiFormat: llm.APIFormatOpenAIChatCompletion, responsesChatTools: true}
		got, err := filterResponsesChatToolMessagesForOutbound(request, outbound)
		require.NoError(t, err)
		require.Same(t, request, got)
	})

	t.Run("filters and pairs all special calls when Chat outbound has no lifecycle adapter", func(t *testing.T) {
		request := newResponsesChatToolFilterRequest()
		outbound := &mockTransformer{apiFormat: llm.APIFormatOpenAIChatCompletion}
		got, err := filterResponsesChatToolMessagesForOutbound(request, outbound)
		require.NoError(t, err)
		require.NotSame(t, request, got)
		require.Len(t, got.Messages, 2)
		require.Len(t, got.Messages[0].ToolCalls, 1)
		require.Equal(t, llm.ToolTypeFunction, got.Messages[0].ToolCalls[0].Type)
		require.NotNil(t, got.Messages[1].ToolCallID)
		require.Equal(t, "call_function_1", *got.Messages[1].ToolCallID)
		require.Len(t, got.Tools, 1)
		require.Equal(t, "get_weather", got.Tools[0].Function.Name)
		require.NotNil(t, got.ToolChoice)
		require.Equal(t, "auto", lo.FromPtr(got.ToolChoice.ToolChoice))
		require.False(t, got.ToolChoice.AllowedToolsSet)

		require.Len(t, request.Tools, 5)
		require.Len(t, request.Messages, 5)
		require.Len(t, request.Messages[0].ToolCalls, 4)
		require.True(t, request.ToolChoice.AllowedToolsSet)
	})

	t.Run("does not filter when outbound is responses", func(t *testing.T) {
		request := newResponsesChatToolFilterRequest()
		got, err := filterResponsesChatToolMessagesForOutbound(request, &mockTransformer{
			apiFormat: llm.APIFormatOpenAIResponse, responsesCapabilities: nativeResponsesCapabilities,
		})
		require.NoError(t, err)
		require.Same(t, request, got)
	})

	t.Run("does not filter when inbound is not responses", func(t *testing.T) {
		request := newResponsesChatToolFilterRequest()
		request.APIFormat = llm.APIFormatOpenAIChatCompletion
		got, err := filterResponsesChatToolMessagesForOutbound(request, &mockTransformer{apiFormat: llm.APIFormatOpenAIChatCompletion})
		require.NoError(t, err)
		require.Same(t, request, got)
	})
}

func TestFilterResponsesChatToolMessagesForOutbound_RequestCapabilityMatrix(t *testing.T) {
	type capabilityKind string
	const (
		capabilityNative    capabilityKind = "native"
		capabilityLifecycle capabilityKind = "chat_lifecycle"
		capabilityPlain     capabilityKind = "plain_chat"
	)

	tests := []struct {
		name        string
		apiFormat   llm.APIFormat
		requestType llm.RequestType
		capability  capabilityKind
		wantSame    bool
		wantSpecial bool
	}{
		{name: "Responses to native", apiFormat: llm.APIFormatOpenAIResponse, requestType: "", capability: capabilityNative, wantSame: true, wantSpecial: true},
		{name: "Responses to Chat lifecycle", apiFormat: llm.APIFormatOpenAIResponse, requestType: "", capability: capabilityLifecycle, wantSame: true, wantSpecial: true},
		{name: "Responses to plain Chat", apiFormat: llm.APIFormatOpenAIResponse, requestType: "", capability: capabilityPlain, wantSpecial: false},
		{name: "Compact to native", apiFormat: llm.APIFormatOpenAIResponseCompact, requestType: llm.RequestTypeCompact, capability: capabilityNative, wantSame: true, wantSpecial: true},
		{name: "Compact to Chat lifecycle", apiFormat: llm.APIFormatOpenAIResponseCompact, requestType: llm.RequestTypeCompact, capability: capabilityLifecycle, wantSame: true, wantSpecial: true},
		{name: "Compact to plain Chat", apiFormat: llm.APIFormatOpenAIResponseCompact, requestType: llm.RequestTypeCompact, capability: capabilityPlain, wantSpecial: false},
		{name: "Chat to native", apiFormat: llm.APIFormatOpenAIChatCompletion, requestType: "", capability: capabilityNative, wantSame: true, wantSpecial: true},
		{name: "Chat to Chat lifecycle", apiFormat: llm.APIFormatOpenAIChatCompletion, requestType: "", capability: capabilityLifecycle, wantSame: true, wantSpecial: true},
		{name: "Chat to plain Chat", apiFormat: llm.APIFormatOpenAIChatCompletion, requestType: "", capability: capabilityPlain, wantSame: true, wantSpecial: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customCallID := "custom_1"
			plainCallID := "plain_1"
			request := &llm.Request{
				APIFormat:   tt.apiFormat,
				RequestType: tt.requestType,
				Messages: []llm.Message{
					{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("run")}},
					{Role: "assistant", ToolCalls: []llm.ToolCall{
						{
							ID: customCallID, Type: llm.ToolTypeResponsesCustomTool,
							ResponseCustomToolCall: &llm.ResponseCustomToolCall{CallID: customCallID, Name: "apply_patch", Input: "patch"},
						},
						{ID: plainCallID, Type: llm.ToolTypeFunction, Function: llm.FunctionCall{Name: "lookup", Arguments: `{}`}},
					}},
					{Role: "tool", ToolCallID: &customCallID, Content: llm.MessageContent{Content: lo.ToPtr("patched")}},
					{Role: "tool", ToolCallID: &plainCallID, Content: llm.MessageContent{Content: lo.ToPtr("looked up")}},
				},
				Tools: []llm.Tool{
					{Type: llm.ToolTypeResponsesCustomTool, ResponseCustomTool: &llm.ResponseCustomTool{Name: "apply_patch"}},
					{Type: llm.ToolTypeFunction, Function: llm.Function{Name: "lookup"}},
				},
			}
			outbound := &mockTransformer{
				apiFormat: llm.APIFormatOpenAIChatCompletion,
				responsesCapabilities: func(*llm.Request) transformer.ResponsesRequestCapabilities {
					return transformer.ResponsesRequestCapabilities{
						NativeResponses:   tt.capability == capabilityNative,
						ChatToolLifecycle: tt.capability == capabilityLifecycle,
					}
				},
			}

			got, err := filterResponsesChatToolMessagesForOutbound(request, outbound)
			require.NoError(t, err)
			if tt.wantSame {
				require.Same(t, request, got)
			} else {
				require.NotSame(t, request, got)
			}

			callIDs := make(map[string]struct{})
			outputIDs := make(map[string]struct{})
			for _, message := range got.Messages {
				for _, call := range message.ToolCalls {
					callIDs[call.ID] = struct{}{}
				}
				if message.ToolCallID != nil {
					outputIDs[*message.ToolCallID] = struct{}{}
				}
			}
			for outputID := range outputIDs {
				require.Contains(t, callIDs, outputID, "tool output must retain a matching assistant call")
			}
			_, hasCustomCall := callIDs[customCallID]
			require.Equal(t, tt.wantSpecial, hasCustomCall)
			_, hasPlainCall := callIDs[plainCallID]
			require.True(t, hasPlainCall)
			_, hasCustomOutput := outputIDs[customCallID]
			require.Equal(t, tt.wantSpecial, hasCustomOutput)
			_, hasPlainOutput := outputIDs[plainCallID]
			require.True(t, hasPlainOutput)

			toolTypes := lo.Map(got.Tools, func(tool llm.Tool, _ int) string { return tool.Type })
			require.Equal(t, tt.wantSpecial, lo.Contains(toolTypes, llm.ToolTypeResponsesCustomTool))
			require.True(t, lo.Contains(toolTypes, llm.ToolTypeFunction))
		})
	}
}
