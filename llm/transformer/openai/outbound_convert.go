package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
	"github.com/looplj/axonhub/llm/transformer/shared"
)

// RequestFromLLM creates OpenAI Request from unified llm.Request with reasoning
// field configuration. When the request has no explicit prompt cache key, ctx's
// session ID is used as a fallback when available.
func RequestFromLLM(ctx context.Context, r *llm.Request, reasoningField ReasoningField) (*Request, error) {
	if r == nil {
		return nil, nil
	}
	// The plain codec cannot represent Responses-only tool lifecycle state.
	// Wrapper transformers call this codec directly, so downgrade here to keep
	// specialized calls from being serialized as invalid Chat history entries
	// (empty names/arguments) that strict providers reject.
	if isResponsesAPIFormat(r.APIFormat) {
		var err error
		r, err = shared.DowngradeResponsesChatToolLifecycle(r)
		if err != nil {
			return nil, fmt.Errorf("failed to downgrade Responses tool lifecycle: %w", err)
		}
	}
	req := requestFromLLMBase(ctx, r)
	req.Messages = lo.Map(r.Messages, func(m llm.Message, _ int) Message {
		message := MessageFromLLMWithConfig(m, reasoningField)
		// Tool call indexes identify positions within one assistant request
		// message. Normalize history here without rewriting response deltas,
		// whose indexes must remain stable across streaming chunks.
		for index := range message.ToolCalls {
			message.ToolCalls[index].Index = index
		}
		return message
	})
	// Chat Completions accepts a single system message; strict OpenAI-compatible
	// upstreams (notably domestic model gateways) reject the multiples that
	// Claude Code produces when it sends the system prompt as an array. Merge
	// them, mirroring the Responses outbound which folds system messages into a
	// single `instructions` string.
	req.Messages = mergeSystemMessages(req.Messages)
	req.Tools = lo.FilterMap(r.Tools, func(tool llm.Tool, _ int) (Tool, bool) {
		return ToolFromLLM(tool), tool.Type == llm.ToolTypeFunction
	})
	if r.ToolChoice != nil {
		req.ToolChoice = &ToolChoice{ToolChoice: r.ToolChoice.ToolChoice}
		if r.ToolChoice.NamedToolChoice != nil {
			req.ToolChoice.NamedToolChoice = &NamedToolChoice{
				Type: r.ToolChoice.NamedToolChoice.Type,
				Function: ToolFunction{
					Name: r.ToolChoice.NamedToolChoice.Function.Name,
				},
			}
		}
	}
	if len(req.Tools) == 0 {
		req.ParallelToolCalls = nil
	}
	return req, nil
}

// RequestFromLLMWithResponsesTools converts a request while retaining metadata
// needed to restore Responses-only tool calls. Provider-specific Chat codecs use
// this path when they advertise Responses chat-tool lifecycle support.

// requestFromLLMBase converts fields shared by plain and Responses-adapted Chat requests.

// requestFromLLMWithResponsesToolAdapter converts a request and preserves reversible tool mappings.

// mergeSystemMessages collapses all system-role messages into one at the
// position of the first, dropping the rest. The Chat Completions spec allows
// a single system message, and strict OpenAI-compatible upstreams reject
// extras with "System message must be at the beginning".
func mergeSystemMessages(msgs []Message) []Message {
	var (
		systemCount int
		firstSystem = -1
		systemText  strings.Builder
	)

	for i, m := range msgs {
		if m.Role != "system" {
			continue
		}
		if firstSystem == -1 {
			firstSystem = i
		}
		systemCount++
		for _, text := range messageTextParts(m) {
			if systemText.Len() > 0 {
				systemText.WriteString("\n\n")
			}
			systemText.WriteString(text)
		}
	}

	if systemCount == 0 {
		return msgs
	}

	merged := make([]Message, 0, len(msgs)-systemCount+1)
	mergedSystem := msgs[firstSystem]
	if systemCount > 1 {
		mergedSystem.Content = MessageContent{Content: lo.ToPtr(systemText.String())}
	}
	merged = append(merged, mergedSystem)
	for _, m := range msgs {
		if m.Role != "system" {
			merged = append(merged, m)
		}
	}
	return merged
}

// messageTextParts extracts the text segments of a message. MultipleContent
// takes precedence over the scalar Content (matching MessageContent.MarshalJSON
// and the documented mutual-exclusivity rule); the scalar is read only when
// there are no parts, so a message carrying both representations is not
// double-counted.
func messageTextParts(m Message) []string {
	if len(m.Content.MultipleContent) > 0 {
		parts := make([]string, 0, len(m.Content.MultipleContent))
		for _, p := range m.Content.MultipleContent {
			if p.Type == "text" && p.Text != nil {
				parts = append(parts, *p.Text)
			}
		}
		return parts
	}
	if m.Content.Content != nil {
		return []string{*m.Content.Content}
	}
	return nil
}

// MessageFromLLM creates OpenAI Message from unified llm.Message.
// Defaults to ReasoningFieldAll to preserve both reasoning fields.
func MessageFromLLM(m llm.Message) Message {
	return MessageFromLLMWithConfig(m, ReasoningFieldAll)
}

// MessageFromLLMWithConfig creates OpenAI Message from unified llm.Message with reasoning field configuration.
func MessageFromLLMWithConfig(m llm.Message, reasoningField ReasoningField) Message {
	var reasoningContent, reasoning *string

	// Apply reasoning field configuration
	switch reasoningField {
	case ReasoningFieldContent:
		// Only use reasoning_content field
		// Prefer ReasoningContent, fallback to Reasoning if ReasoningContent is nil
		reasoningContent = m.ReasoningContent
		if reasoningContent == nil && m.Reasoning != nil {
			reasoningContent = m.Reasoning
		}
		reasoning = nil
	case ReasoningFieldReasoning:
		// Only use reasoning field
		// Prefer Reasoning, fallback to ReasoningContent if Reasoning is nil
		reasoning = m.Reasoning
		if reasoning == nil && m.ReasoningContent != nil {
			reasoning = m.ReasoningContent
		}
		reasoningContent = nil
	case ReasoningFieldNone:
		// Strip all reasoning fields
		reasoningContent = nil
		reasoning = nil
	default: // ReasoningFieldAll
		// Preserve both reasoning fields with sync logic
		reasoningContent = m.ReasoningContent
		reasoning = m.Reasoning

		// Sync: if one field has value and the other is nil/empty, copy the value
		if reasoningContent == nil && reasoning != nil && *reasoning != "" {
			reasoningContent = reasoning
		}
		if reasoning == nil && reasoningContent != nil && *reasoningContent != "" {
			reasoning = reasoningContent
		}
	}

	// Build the Message with determined fields
	msg := Message{
		Role:             m.Role,
		Name:             m.Name,
		Refusal:          m.Refusal,
		ToolCallID:       m.ToolCallID,
		ReasoningContent: reasoningContent,
		Reasoning:        reasoning,
	}

	if m.Audio != nil {
		msg.Audio = &OutputAudio{
			ID:         m.Audio.ID,
			Data:       m.Audio.Data,
			ExpiresAt:  m.Audio.ExpiresAt,
			Transcript: m.Audio.Transcript,
		}
	}

	// Convert Content
	msg.Content = MessageContentFromLLM(m.Content)

	// Chat Completions accepts a string for the tool role; a multi-part tool
	// result (e.g. a Responses function_call_output with several text items)
	// would otherwise serialize as an array and be rejected with a 400.
	if m.Role == "tool" {
		msg.Content = flattenChatToolContent(msg.Content)
	}

	// Convert ToolCalls
	if m.ToolCalls != nil {
		msg.ToolCalls = lo.Map(m.ToolCalls, func(tc llm.ToolCall, _ int) ToolCall {
			return ToolCallFromLLM(tc)
		})
	}

	// Assistant turns can carry no content: turns that only request tool calls,
	// turns that only carry reasoning (a Responses reasoning item not followed by
	// text or a call), and messages whose parts were all filtered out (e.g.
	// compaction) are left with an empty part list. These cases would reach the
	// wire as a missing or null content field, which the OpenAI spec permits but
	// stricter OpenAI-compatible upstreams reject because their schema only
	// accepts a string or an array. Normalize to an empty string, which every
	// implementation accepts and OpenAI treats as no content.
	if msg.Role == "assistant" && msg.Content.Content == nil && len(msg.Content.MultipleContent) == 0 {
		msg.Content = MessageContent{Content: lo.ToPtr("")}
	}

	// Convert Annotations
	if len(m.Annotations) > 0 {
		msg.Annotations = lo.Map(m.Annotations, func(a llm.Annotation, _ int) Annotation {
			return AnnotationFromLLM(a)
		})
	}

	return msg
}

// AnnotationFromLLM creates OpenAI Annotation from unified llm.Annotation.
func AnnotationFromLLM(a llm.Annotation) Annotation {
	annotation := Annotation{
		Type:       a.Type,
		StartIndex: a.StartIndex,
		EndIndex:   a.EndIndex,
	}

	if a.URLCitation != nil {
		annotation.URLCitation = &URLCitation{
			URL:   a.URLCitation.URL,
			Title: a.URLCitation.Title,
		}
	}

	return annotation
}

// MessageContentFromLLM creates OpenAI MessageContent from unified llm.MessageContent.
func MessageContentFromLLM(c llm.MessageContent) MessageContent {
	content := MessageContent{
		Content: c.Content,
	}

	if c.MultipleContent != nil {
		content.MultipleContent = lo.FilterMap(c.MultipleContent, func(p llm.MessageContentPart, _ int) (MessageContentPart, bool) {
			switch p.Type {
			case "compaction", "compaction_summary":
				return MessageContentPart{}, false
			default:
				return MessageContentPartFromLLM(p), true
			}
		})
	}

	return content
}

// MessageContentPartFromLLM creates OpenAI MessageContentPart from unified llm.MessageContentPart.
func MessageContentPartFromLLM(p llm.MessageContentPart) MessageContentPart {
	part := MessageContentPart{
		Type: normalizeContentPartType(p.Type),
		Text: p.Text,
	}

	if p.ImageURL != nil {
		part.ImageURL = &ImageURL{
			URL:    p.ImageURL.URL,
			Detail: p.ImageURL.Detail,
		}
	}

	if p.VideoURL != nil {
		part.VideoURL = &VideoURL{
			URL: p.VideoURL.URL,
		}
	}

	if p.InputAudio != nil {
		part.InputAudio = &InputAudio{
			Format: p.InputAudio.Format,
			Data:   p.InputAudio.Data,
		}
	}

	if p.Document != nil {
		part.Type = "file"
		part.File = &File{
			FileID:   p.Document.FileID,
			Filename: p.Document.Filename,
		}
		if strings.HasPrefix(p.Document.URL, "data:") {
			part.File.FileData = p.Document.URL
		}
		if part.File.Filename == "" && p.Document.MIMEType == "application/pdf" {
			part.File.Filename = "document.pdf"
		}
	}

	return part
}

func validateChatDocumentParts(messages []llm.Message) error {
	for _, message := range messages {
		for _, part := range message.Content.MultipleContent {
			if part.Type != "document" || part.Document == nil {
				continue
			}
			if part.Document.FileID == "" && part.Document.URL != "" && !strings.HasPrefix(part.Document.URL, "data:") {
				return fmt.Errorf("%w: OpenAI Chat file inputs require file_id or a data URL in file_data", transformer.ErrInvalidRequest)
			}
		}
	}

	return nil
}

// normalizeContentPartType maps Responses-only text part types onto the plain
// "text" type used by Chat Completions. The Responses API distinguishes input
// from output text, but Chat Completions has a single text part type, and strict
// OpenAI-compatible upstreams reject the ones they do not know. Types that Chat
// Completions does understand (image_url, video_url, input_audio, ...) pass through.
func normalizeContentPartType(partType string) string {
	switch partType {
	case "input_text", "output_text":
		return "text"
	default:
		return partType
	}
}

// flattenChatToolContent collapses a Chat tool message's content into a single
// string. Text parts are concatenated in order; parts a Chat tool message
// cannot carry (e.g. images) are dropped, which still beats emitting an array
// that the provider rejects outright.

// ToolFromLLM creates OpenAI Tool from unified llm.Tool.
func ToolFromLLM(t llm.Tool) Tool {
	return Tool{
		Type: t.Type,
		Function: Function{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
			Strict:      t.Function.Strict,
		},
	}
}

// ToolCallFromLLM creates OpenAI ToolCall from unified llm.ToolCall.
func ToolCallFromLLM(tc llm.ToolCall) ToolCall {
	toolCall := ToolCall{
		ID:   tc.ID,
		Type: tc.Type,
		Function: FunctionCall{
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		},
		Index: tc.Index,
	}

	if raw, ok := tc.TransformerMetadata[TransformerMetadataKeyGoogleThoughtSignature].(string); ok && raw != "" {
		toolCall.ExtraContent = &ToolCallExtraContent{
			Google: &ToolCallGoogleExtraContent{
				ThoughtSignature: raw,
			},
		}
	}

	return toolCall
}

// ToLLMResponse converts OpenAI Response to unified llm.Response.
func (r *Response) ToLLMResponse() *llm.Response {
	if r == nil {
		return nil
	}

	resp := &llm.Response{
		ID:                r.ID,
		Object:            r.Object,
		Created:           r.Created,
		Model:             r.Model,
		SystemFingerprint: r.SystemFingerprint,
		ServiceTier:       r.ServiceTier,
	}

	// Convert choices
	resp.Choices = lo.Map(r.Choices, func(c Choice, _ int) llm.Choice {
		return c.ToLLMChoice()
	})

	// Convert usage
	if r.Usage != nil {
		resp.Usage = r.Usage.ToLLMUsage()
	}

	// Convert error
	if r.Error != nil {
		resp.Error = &llm.ResponseError{
			StatusCode: r.Error.StatusCode,
			Detail:     r.Error.Detail,
		}
	}

	// Store citations in TransformerMetadata if present
	if len(r.Citations) > 0 {
		if resp.TransformerMetadata == nil {
			resp.TransformerMetadata = make(map[string]any)
		}

		resp.TransformerMetadata[TransformerMetadataKeyCitations] = r.Citations
	}

	return resp
}

// ToLLMChoice converts OpenAI Choice to unified llm.Choice.
func (c Choice) ToLLMChoice() llm.Choice {
	choice := llm.Choice{
		Index:        c.Index,
		FinishReason: c.FinishReason,
	}

	if c.Message != nil {
		msg := c.Message.ToLLMMessage()
		choice.Message = &msg
	}

	if c.Delta != nil {
		delta := c.Delta.ToLLMMessage()
		choice.Delta = &delta
	}

	if c.Logprobs != nil {
		choice.Logprobs = &llm.LogprobsContent{
			Content: lo.Map(c.Logprobs.Content, func(t TokenLogprob, _ int) llm.TokenLogprob {
				return llm.TokenLogprob{
					Token:   t.Token,
					Logprob: t.Logprob,
					Bytes:   t.Bytes,
					TopLogprobs: lo.Map(t.TopLogprobs, func(tl TopLogprob, _ int) llm.TopLogprob {
						return llm.TopLogprob{
							Token:   tl.Token,
							Logprob: tl.Logprob,
							Bytes:   tl.Bytes,
						}
					}),
				}
			}),
		}
	}

	return choice
}
