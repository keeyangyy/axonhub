package openai

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/transformer"
	"github.com/looplj/axonhub/llm/transformer/shared"
	"github.com/samber/lo"
)

// RequestFromLLMWithResponsesTools (fork; moved from outbound_convert.go)
func RequestFromLLMWithResponsesTools(
	ctx context.Context,
	r *llm.Request,
	reasoningField ReasoningField,
) (*Request, map[string]any, error) {
	if r == nil {
		return nil, nil, nil
	}
	if !isResponsesAPIFormat(r.APIFormat) {
		req, err := RequestFromLLM(ctx, r, reasoningField)
		return req, nil, err
	}

	req, adapter, err := requestFromLLMWithResponsesToolAdapter(ctx, r, reasoningField)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", transformer.ErrInvalidRequest, err)
	}
	if adapter == nil {
		return req, nil, nil
	}

	return req, responsesChatToolMetadata(r.TransformerMetadata, adapter), nil
}

// responsesChatToolMetadata (fork; moved from outbound_convert.go)
func responsesChatToolMetadata(existing map[string]any, adapter *responsesChatToolAdapter) map[string]any {
	if adapter == nil {
		return maps.Clone(existing)
	}

	metadata := make(map[string]any, len(existing)+4)
	maps.Copy(metadata, existing)
	metadata[responsesChatStrictFinishMetadataKey] = true
	if mappings := adapter.mappings(); len(mappings) > 0 {
		metadata[ResponsesChatToolMappingsMetadataKey] = mappings
	}
	if catalog := adapter.catalog(); len(catalog) > 0 {
		metadata[ResponsesChatToolCatalogMetadataKey] = catalog
	}
	if len(adapter.warnings) > 0 {
		metadata[responsesChatToolWarningsMetadataKey] = append([]string(nil), adapter.warnings...)
	}
	return metadata
}

// requestFromLLMBase (fork; moved from outbound_convert.go)
func requestFromLLMBase(ctx context.Context, r *llm.Request) *Request {
	req := &Request{
		Model:               r.Model,
		FrequencyPenalty:    r.FrequencyPenalty,
		Logprobs:            r.Logprobs,
		MaxCompletionTokens: r.MaxCompletionTokens,
		MaxTokens:           r.MaxTokens,
		PresencePenalty:     r.PresencePenalty,
		Seed:                r.Seed,
		Store:               r.Store,
		Temperature:         r.Temperature,
		TopLogprobs:         r.TopLogprobs,
		TopP:                r.TopP,
		PromptCacheKey:      r.PromptCacheKey,
		SafetyIdentifier:    r.SafetyIdentifier,
		User:                r.User,
		LogitBias:           r.LogitBias,
		Metadata:            r.Metadata,
		Modalities:          r.Modalities,
		ReasoningEffort:     r.ReasoningEffort,
		ServiceTier:         r.ServiceTier,
		Stream:              r.Stream,
		ParallelToolCalls:   r.ParallelToolCalls,
		Verbosity:           r.Verbosity,
	}

	if ctx != nil && lo.FromPtr(req.PromptCacheKey) == "" {
		if sessionID, ok := shared.GetSessionID(ctx); ok && sessionID != "" {
			req.PromptCacheKey = lo.ToPtr(sessionID)
		}
	}

	// Convert Stop
	if r.Stop != nil {
		req.Stop = &Stop{Stop: r.Stop.Stop, MultipleStop: r.Stop.MultipleStop}
	}
	if r.StreamOptions != nil {
		req.StreamOptions = &StreamOptions{IncludeUsage: r.StreamOptions.IncludeUsage}
	}
	if r.ResponseFormat != nil {
		req.ResponseFormat = &ResponseFormat{
			Type: r.ResponseFormat.Type, JSONSchema: r.ResponseFormat.JSONSchema,
		}
	}
	return req
}

// requestFromLLMWithResponsesToolAdapter (fork; moved from outbound_convert.go)
func requestFromLLMWithResponsesToolAdapter(ctx context.Context, r *llm.Request, reasoningField ReasoningField) (*Request, *responsesChatToolAdapter, error) {
	if r == nil {
		return nil, nil, nil
	}
	toolAdapter := newResponsesChatToolAdapter(r.Tools)
	degradedToolChoice := toolAdapter.degradeUnsupportedRawToolSelector(r)

	req := requestFromLLMBase(ctx, r)

	// Build the callable catalog before converting history so specialized calls
	// resolve through the same stable names as the current tool declarations.
	req.Tools = toolAdapter.filterAllowedTools(toolAdapter.convertTools(r.Tools), r.ToolChoice)

	// Convert messages. Responses can retain assistant-only metadata such as
	// encrypted reasoning or compaction items that Chat Completions cannot
	// represent. Once those fields are stripped, omit the empty assistant
	// message instead of sending an invalid history entry to the provider.
	droppedEmptyAssistants := 0
	req.Messages = lo.FilterMap(r.Messages, func(m llm.Message, _ int) (Message, bool) {
		converted := toolAdapter.convertMessage(m, reasoningField)
		if !hasChatAssistantPayload(converted) {
			droppedEmptyAssistants++
			return Message{}, false
		}
		return converted, true
	})
	if droppedEmptyAssistants > 0 {
		toolAdapter.addWarningf(
			"empty_assistant_message: dropped %d history message(s) with no Chat-compatible payload",
			droppedEmptyAssistants,
		)
	}

	// Convert ToolChoice
	if !degradedToolChoice {
		req.ToolChoice = toolAdapter.convertToolChoice(r.ToolChoice)
	}

	if len(req.Tools) == 0 {
		req.ParallelToolCalls = nil
		if req.ToolChoice != nil && req.ToolChoice.ToolChoice != nil {
			switch *req.ToolChoice.ToolChoice {
			case "auto", "none":
				req.ToolChoice = nil
			case "required":
				toolAdapter.setError(fmt.Errorf("unsupported_tool_choice: required tool choice has no callable tools after Responses-to-Chat conversion"))
			}
		}
	}

	if toolAdapter.err != nil {
		return nil, toolAdapter, toolAdapter.err
	}
	return req, toolAdapter, nil
}

// flattenChatToolContent (fork; moved from outbound_convert.go)
func flattenChatToolContent(content MessageContent) MessageContent {
	if len(content.MultipleContent) == 0 {
		return content
	}

	var builder strings.Builder
	for _, part := range content.MultipleContent {
		if part.Type == "text" && part.Text != nil {
			builder.WriteString(*part.Text)
		}
	}

	text := builder.String()

	return MessageContent{Content: &text}
}

// isResponsesAPIFormat (fork; moved from outbound.go)
func isResponsesAPIFormat(format llm.APIFormat) bool {
	return llm.IsOpenAIResponsesFormat(format)
}

// responsesChatToolMappings (fork; moved from outbound.go)
func responsesChatToolMappings(req *httpclient.Request) map[string]responsesChatToolMapping {
	if req == nil || req.TransformerMetadata == nil {
		return nil
	}
	mappings, _ := req.TransformerMetadata[ResponsesChatToolMappingsMetadataKey].(map[string]responsesChatToolMapping)
	return mappings
}

// responsesChatToolCatalog (fork; moved from outbound.go)
func responsesChatToolCatalog(req *httpclient.Request) []string {
	if req == nil || req.TransformerMetadata == nil {
		return nil
	}
	catalog, _ := req.TransformerMetadata[ResponsesChatToolCatalogMetadataKey].([]string)
	return catalog
}
