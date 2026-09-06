package responses

import (
	"bytes"
	"encoding/json"
	"github.com/looplj/axonhub/llm"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
)

func TestConvertToolChoicePreservesAllowedTools(t *testing.T) {
	required := "required"
	converted := convertToolChoice(&llm.ToolChoice{
		ToolChoice: &required, AllowedToolsSet: true,
		AllowedTools: []llm.ToolOption{
			{Type: "function", Name: "lookup"},
			{Type: "custom", Name: "apply_patch"},
		},
	})
	require.NotNil(t, converted)
	require.Equal(t, "allowed_tools", lo.FromPtr(converted.Type))
	require.Equal(t, "required", lo.FromPtr(converted.Mode))
	require.Equal(t, []ToolOption{
		{Type: "function", Name: "lookup"},
		{Type: "custom", Name: "apply_patch"},
	}, converted.Tools)
}

func TestConvertToolChoicePrimitiveMatrix(t *testing.T) {
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
				converted := convertToolChoice(&llm.ToolChoice{
					NamedToolChoice: &llm.NamedToolChoice{
						Type:     tt.toolType,
						Function: llm.ToolFunction{Name: tt.name},
					},
				})
				require.NotNil(t, converted)
				require.Nil(t, converted.Mode)
				require.Equal(t, tt.toolType, lo.FromPtr(converted.Type))
				require.Equal(t, tt.name, lo.FromPtr(converted.Name))
				require.Nil(t, converted.Tools)
			})
		}
	})

	t.Run("allowed selectors retain same name across primitive types", func(t *testing.T) {
		for _, mode := range []string{"auto", "required"} {
			t.Run(mode, func(t *testing.T) {
				converted := convertToolChoice(&llm.ToolChoice{
					ToolChoice:      lo.ToPtr(mode),
					AllowedToolsSet: true,
					AllowedTools: []llm.ToolOption{
						{Type: "function", Name: "same"},
						{Type: "custom", Name: "same"},
						{Type: "namespace", Name: "workspace"},
						{Type: "tool_search", Name: "discover"},
						{Type: "future_client_tool", Name: "later"},
						{Type: "future_server_tool", Name: "hosted"},
					},
				})
				require.NotNil(t, converted)
				require.Equal(t, "allowed_tools", lo.FromPtr(converted.Type))
				require.Equal(t, mode, lo.FromPtr(converted.Mode))
				require.Nil(t, converted.Name)
				require.Equal(t, []ToolOption{
					{Type: "function", Name: "same"},
					{Type: "custom", Name: "same"},
					{Type: "namespace", Name: "workspace"},
					{Type: "tool_search", Name: "discover"},
					{Type: "future_client_tool", Name: "later"},
					{Type: "future_server_tool", Name: "hosted"},
				}, converted.Tools)
			})
		}
	})
}

func TestConvertToolSearchOutputMalformedContentDegradesWithWarning(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	result := convertToolMessageWithType(llm.Message{
		Role:       "tool",
		ToolCallID: lo.ToPtr("call_search"),
		Content:    llm.MessageContent{Content: lo.ToPtr(`[{"type":"function"}`)},
	}, "tool_search_output")

	require.Equal(t, "tool_search_output", result.Type)
	require.NotNil(t, result.Tools)
	require.Empty(t, result.Tools)
	require.Contains(t, logs.String(), "failed to decode tool_search_output tools")
	require.Contains(t, logs.String(), "call_search")
}

func TestConvertToolSearchOutputNullContentDegradesWithWarning(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	result := convertToolMessageWithType(llm.Message{
		Role:       "tool",
		ToolCallID: lo.ToPtr("call_search_null"),
		Content:    llm.MessageContent{Content: lo.ToPtr(`null`)},
	}, "tool_search_output")

	require.NotNil(t, result.Tools)
	require.Empty(t, result.Tools)
	require.Contains(t, logs.String(), "expected a JSON array")
	require.Contains(t, logs.String(), "call_search_null")
}

func TestConvertToolSearchOutputEmptyContentSkipsWarning(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	result := convertToolMessageWithType(llm.Message{
		Role:       "tool",
		ToolCallID: lo.ToPtr("call_search_empty"),
		Content:    llm.MessageContent{Content: lo.ToPtr("   ")},
	}, "tool_search_output")

	require.Equal(t, "tool_search_output", result.Type)
	require.NotNil(t, result.Tools)
	require.Empty(t, result.Tools)
	require.NotContains(t, logs.String(), "failed to decode tool_search_output tools")
}

func TestConvertToolSearchOutputJoinsMultipleTextParts(t *testing.T) {
	result := convertToolMessageWithType(llm.Message{
		Role:       "tool",
		ToolCallID: lo.ToPtr("call_search_parts"),
		Content: llm.MessageContent{MultipleContent: []llm.MessageContentPart{
			{Type: "text", Text: lo.ToPtr(`[{"type":"function"`)},
			{Type: "text", Text: lo.ToPtr(`,"name":"lookup"}]`)},
		}},
	}, "tool_search_output")

	require.Equal(t, "tool_search_output", result.Type)
	require.NotNil(t, result.Tools)
	require.Len(t, result.Tools, 1)
	require.Equal(t, "function", result.Tools[0].Type)
	require.Equal(t, "lookup", result.Tools[0].Name)
}

func TestAppendResponseWebSearchCallMetadata_DoesNotMutateSourceSliceBackingArray(t *testing.T) {
	original := make([]Item, 1, 2)
	original[0] = Item{ID: "existing", Type: "web_search_call"}
	metadata := map[string]any{
		responsesWebSearchCallsTransformerMetadataKey: original,
	}

	appendResponseWebSearchCallMetadata(metadata, Item{
		ID:   "new",
		Type: "web_search_call",
		Action: NewWebSearchAction(&WebSearchAction{
			Type:  "search",
			Query: "agents",
		}),
	})

	extendedOriginal := original[:2]
	require.Empty(t, extendedOriginal[1], "append must not write through caller-owned spare capacity")
	stored, ok := metadata[responsesWebSearchCallsTransformerMetadataKey].([]Item)
	require.True(t, ok)
	require.Len(t, stored, 2)
	require.Equal(t, "existing", stored[0].ID)
	require.Equal(t, "new", stored[1].ID)
}

func TestResponseToolSearchOutputDefinition(t *testing.T) {
	t.Run("malformed tool search parameters return error", func(t *testing.T) {
		_, ok, err := responseToolSearchOutputDefinition(llm.Tool{
			Type: llm.ToolTypeResponsesToolSearch,
			ResponseToolSearch: &llm.ResponseToolSearch{
				Execution:  "client",
				Parameters: json.RawMessage(`{"query":`),
			},
		})
		require.Error(t, err)
		require.False(t, ok)
	})

	t.Run("missing tool search definition is skipped", func(t *testing.T) {
		_, ok, err := responseToolSearchOutputDefinition(llm.Tool{Type: llm.ToolTypeResponsesToolSearch})
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("valid tool search parameters decode", func(t *testing.T) {
		tool, ok, err := responseToolSearchOutputDefinition(llm.Tool{
			Type: llm.ToolTypeResponsesToolSearch,
			ResponseToolSearch: &llm.ResponseToolSearch{
				Execution:   "client",
				Description: "Find tools",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
		})
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "tool_search", tool.Type)
		require.Equal(t, "client", tool.Execution)
		require.Equal(t, "Find tools", tool.Description)
		require.Equal(t, "object", tool.Parameters["type"])
	})

	t.Run("function branch", func(t *testing.T) {
		tool, ok, err := responseToolSearchOutputDefinition(llm.Tool{
			Type:     llm.ToolTypeFunction,
			Function: llm.Function{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)},
		})
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "function", tool.Type)
		require.Equal(t, "lookup", tool.Name)
	})

	t.Run("image generation branch", func(t *testing.T) {
		tool, ok, err := responseToolSearchOutputDefinition(llm.Tool{
			Type:            llm.ToolTypeImageGeneration,
			ImageGeneration: &llm.ImageGeneration{Model: "gpt-image-1", OutputFormat: "png"},
		})
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "image_generation", tool.Type)
		require.Equal(t, "gpt-image-1", tool.Model)
		require.Equal(t, "png", tool.OutputFormat)
	})

	t.Run("web search branch", func(t *testing.T) {
		tool, ok, err := responseToolSearchOutputDefinition(llm.Tool{
			Type:      llm.ToolTypeWebSearch,
			WebSearch: &llm.WebSearch{AllowedDomains: []string{"example.com"}},
		})
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "web_search", tool.Type)
		require.NotNil(t, tool.Filters)
		require.Equal(t, []string{"example.com"}, tool.Filters.AllowedDomains)
	})

	t.Run("custom branch", func(t *testing.T) {
		tool, ok, err := responseToolSearchOutputDefinition(llm.Tool{
			Type: llm.ToolTypeResponsesCustomTool,
			ResponseCustomTool: &llm.ResponseCustomTool{
				Name:        "apply_patch",
				Description: "Apply patch",
				Format:      &llm.ResponseCustomToolFormat{Type: "grammar", Syntax: "ebnf", Definition: "root = x"},
			},
		})
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "custom", tool.Type)
		require.Equal(t, "apply_patch", tool.Name)
		require.Equal(t, "Apply patch", tool.Description)
		require.NotNil(t, tool.Format)
		require.Equal(t, "grammar", tool.Format.Type)
		require.Equal(t, "ebnf", tool.Format.Syntax)
		require.Equal(t, "root = x", tool.Format.Definition)
	})

	t.Run("unsupported type is skipped", func(t *testing.T) {
		_, ok, err := responseToolSearchOutputDefinition(llm.Tool{Type: llm.ToolTypeResponsesOpaqueTool})
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestSynchronizeToolSearchOutputMessagesDefinitionSync(t *testing.T) {
	callID := "call_search_sync"
	newMessages := func() []llm.Message {
		return []llm.Message{
			{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{
					{
						ID:                     callID,
						ResponseToolSearchCall: &llm.ResponseToolSearchCall{CallID: callID},
					},
				},
			},
			{
				Role:       "tool",
				ToolCallID: lo.ToPtr(callID),
				Content:    llm.MessageContent{Content: lo.ToPtr(`{"result":"raw search output"}`)},
			},
		}
	}
	requestExt := &llm.OpenAIResponsesRequestExtensions{
		RawInputItems: []llm.OpenAIResponsesRawFragment{
			{Type: "tool_search_output", CallID: callID, OriginalIndex: 4},
		},
	}

	t.Run("unconvertible origin definitions keep original content", func(t *testing.T) {

		tools := []llm.Tool{
			{
				Type: llm.ToolTypeResponsesOpaqueTool,
				ResponseOpaqueTool: &llm.ResponseOpaqueTool{
					SourceType: "future_hosted_tool",
					Name:       "discover",
				},
				ResponsesOrigin:       "tool_search_output",
				ResponsesOriginCallID: callID,
				ResponsesRawID:        "input:4:0",
			},
		}

		result, err := synchronizeToolSearchOutputMessages(newMessages(), tools, requestExt)
		require.NoError(t, err)
		require.Len(t, result, 2)
		require.Equal(t, `{"result":"raw search output"}`, lo.FromPtr(result[1].Content.Content))
	})

	t.Run("absent origin definitions clear content", func(t *testing.T) {
		result, err := synchronizeToolSearchOutputMessages(newMessages(), nil, requestExt)
		require.NoError(t, err)
		require.Len(t, result, 2)
		require.Equal(t, "[]", lo.FromPtr(result[1].Content.Content))
	})

	t.Run("matching definitions replace content", func(t *testing.T) {
		tools := []llm.Tool{
			{
				Type:                  llm.ToolTypeFunction,
				Function:              llm.Function{Name: "lookup", Description: "Look things up"},
				ResponsesOrigin:       "tool_search_output",
				ResponsesOriginCallID: callID,
				ResponsesRawID:        "input:4:0",
			},
		}

		result, err := synchronizeToolSearchOutputMessages(newMessages(), tools, requestExt)
		require.NoError(t, err)
		require.Len(t, result, 2)

		content := lo.FromPtr(result[1].Content.Content)
		require.NotEqual(t, `{"result":"raw search output"}`, content)

		var definitions []Tool
		require.NoError(t, json.Unmarshal([]byte(content), &definitions))
		require.Len(t, definitions, 1)
		require.Equal(t, "function", definitions[0].Type)
		require.Equal(t, "lookup", definitions[0].Name)
	})
}
