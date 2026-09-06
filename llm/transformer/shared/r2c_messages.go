package shared

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/looplj/axonhub/llm"
)

// FilterOutResponsesChatToolLifecycleMessages (fork; moved from messages.go)
func FilterOutResponsesChatToolLifecycleMessages(messages []llm.Message) []llm.Message {
	return filterOutToolLifecycleMessages(messages, func(toolCall llm.ToolCall) bool {
		return toolCall.Type == llm.ToolTypeResponsesCustomTool ||
			toolCall.ResponseCustomToolCall != nil ||
			toolCall.Type == llm.ToolTypeResponsesToolSearch ||
			toolCall.ResponseToolSearchCall != nil ||
			toolCall.Function.Namespace != ""
	})
}

// filterOutToolLifecycleMessages (fork; moved from messages.go)
func filterOutToolLifecycleMessages(messages []llm.Message, shouldRemove func(llm.ToolCall) bool) []llm.Message {
	if len(messages) == 0 {
		return nil
	}

	type lifecycleOccurrence struct {
		messageIndex int
		remove       bool
	}
	occurrences := make(map[string][]lifecycleOccurrence)
	for messageIndex, msg := range messages {
		for _, toolCall := range msg.ToolCalls {
			remove := shouldRemove(toolCall)
			ids := []string{toolCall.ID}
			if toolCall.ResponseCustomToolCall != nil {
				ids = append(ids, toolCall.ResponseCustomToolCall.CallID)
			}
			if toolCall.ResponseToolSearchCall != nil {
				ids = append(ids, toolCall.ResponseToolSearchCall.CallID)
			}
			seen := make(map[string]struct{}, len(ids))
			for _, id := range ids {
				if id == "" {
					continue
				}
				if _, duplicate := seen[id]; duplicate {
					continue
				}
				seen[id] = struct{}{}
				occurrences[id] = append(occurrences[id], lifecycleOccurrence{messageIndex: messageIndex, remove: remove})
			}
		}
	}
	shouldRemoveOutput := func(id string, messageIndex int) bool {
		matches := occurrences[id]
		if len(matches) == 0 {
			return false
		}
		for _, v := range slices.Backward(matches) {
			if v.messageIndex < messageIndex {
				return v.remove
			}
		}
		return matches[0].remove
	}

	filtered := make([]llm.Message, 0, len(messages))

	for messageIndex, msg := range messages {
		if msg.Role == "tool" && msg.ToolCallID != nil {
			if shouldRemoveOutput(*msg.ToolCallID, messageIndex) {
				continue
			}
		}

		cloned := msg
		if len(msg.ToolCalls) > 0 {
			cloned.ToolCalls = make([]llm.ToolCall, 0, len(msg.ToolCalls))
			for _, toolCall := range msg.ToolCalls {
				if shouldRemove(toolCall) {
					continue
				}
				cloned.ToolCalls = append(cloned.ToolCalls, toolCall)
			}
		}

		if !HasChatCompatibleAssistantPayload(cloned) {
			continue
		}

		filtered = append(filtered, cloned)
	}

	return filtered
}

// HasChatCompatibleAssistantPayload (fork; moved from messages.go)
func HasChatCompatibleAssistantPayload(msg llm.Message) bool {
	if msg.Role != "assistant" {
		return true
	}
	if len(msg.ToolCalls) > 0 {
		return true
	}
	if msg.ReasoningContent != nil && strings.TrimSpace(*msg.ReasoningContent) != "" ||
		msg.Reasoning != nil && strings.TrimSpace(*msg.Reasoning) != "" ||
		strings.TrimSpace(msg.Refusal) != "" || hasOutputAudioPayload(msg.Audio) {
		return true
	}
	if len(msg.Content.MultipleContent) == 0 {
		return msg.Content.Content != nil && strings.TrimSpace(*msg.Content.Content) != ""
	}
	for _, part := range msg.Content.MultipleContent {
		switch part.Type {
		case "text", "input_text", "output_text":
			if part.Text != nil && strings.TrimSpace(*part.Text) != "" {
				return true
			}
		case "image_url":
			if part.ImageURL != nil && strings.TrimSpace(part.ImageURL.URL) != "" {
				return true
			}
		case "video_url":
			if part.VideoURL != nil && strings.TrimSpace(part.VideoURL.URL) != "" {
				return true
			}
		case "input_audio":
			if part.InputAudio != nil && strings.TrimSpace(part.InputAudio.Data) != "" {
				return true
			}
		}
	}
	return false
}

// hasOutputAudioPayload (fork; moved from messages.go)
func hasOutputAudioPayload(audio *llm.OutputAudio) bool {
	return audio != nil && (strings.TrimSpace(audio.ID) != "" || strings.TrimSpace(audio.Data) != "" ||
		strings.TrimSpace(audio.Transcript) != "")
}

// SanitizeChatToolArguments (fork; moved from messages.go)
func SanitizeChatToolArguments(messages []llm.Message) ([]llm.Message, bool) {
	result := messages
	changed := false
	// toolCallsCopied tracks per message whether ToolCalls was already copied
	// into a fresh slice, guarding the copy-on-write below.
	toolCallsCopied := make([]bool, len(messages))

	for messageIndex, message := range messages {
		if message.Role != "assistant" || len(message.ToolCalls) == 0 {
			continue
		}

		for callIndex, call := range message.ToolCalls {
			repaired, ok := repairToolCallArguments(call)
			if !ok {
				continue
			}

			if !changed {
				result = append([]llm.Message(nil), messages...)
				changed = true
			}
			if !toolCallsCopied[messageIndex] {
				result[messageIndex].ToolCalls = append([]llm.ToolCall(nil), message.ToolCalls...)
				toolCallsCopied[messageIndex] = true
			}
			result[messageIndex].ToolCalls[callIndex] = repaired
		}
	}

	return result, changed
}

// repairToolCallArguments (fork; moved from messages.go)
func repairToolCallArguments(call llm.ToolCall) (llm.ToolCall, bool) {
	if call.ResponseToolSearchCall != nil {
		if isValidToolCallArguments(call.ResponseToolSearchCall.Arguments) {
			return call, false
		}
		repairedSearchCall := *call.ResponseToolSearchCall
		repairedSearchCall.Arguments = "{}"
		call.ResponseToolSearchCall = &repairedSearchCall
		call.Function.Arguments = "{}"
		return call, true
	}

	if call.ResponseCustomToolCall != nil || isValidToolCallArguments(call.Function.Arguments) {
		return call, false
	}
	call.Function.Arguments = "{}"
	return call, true
}

// isValidToolCallArguments (fork; moved from messages.go)
func isValidToolCallArguments(arguments string) bool {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" || trimmed == "null" {
		return false
	}
	return json.Valid([]byte(trimmed))
}

// SanitizeChatMessageContent (fork; moved from messages.go)
func SanitizeChatMessageContent(messages []llm.Message) ([]llm.Message, bool) {
	result := make([]llm.Message, 0, len(messages))
	changed := false

	for _, message := range messages {
		sanitized, keep, modified := sanitizeChatMessageContent(message)
		if !keep {
			changed = true
			continue
		}
		if modified {
			changed = true
		}
		result = append(result, sanitized)
	}

	if !changed {
		return messages, false
	}
	return result, true
}

// sanitizeChatMessageContent (fork; moved from messages.go)
func sanitizeChatMessageContent(message llm.Message) (llm.Message, bool, bool) {
	// Tool-call turns are valid with null/empty content on every provider.
	if len(message.ToolCalls) > 0 {
		return message, true, false
	}

	if len(message.Content.MultipleContent) > 0 {
		filtered := make([]llm.MessageContentPart, 0, len(message.Content.MultipleContent))
		for _, part := range message.Content.MultipleContent {
			if visibleChatContentPart(part) {
				filtered = append(filtered, part)
			}
		}
		if len(filtered) == len(message.Content.MultipleContent) {
			return message, true, false
		}
		if len(filtered) > 0 {
			message.Content.MultipleContent = filtered
			return message, true, true
		}
		message.Content.MultipleContent = nil
		if keepReasoningOnlyAssistant(message) {
			return message, true, true
		}
		return substituteEmptyToolOutput(message)
	}

	if message.Content.Content != nil && strings.TrimSpace(*message.Content.Content) != "" {
		return message, true, false
	}

	if keepReasoningOnlyAssistant(message) {
		return message, true, false
	}
	return substituteEmptyToolOutput(message)
}

// keepReasoningOnlyAssistant (fork; moved from messages.go)
func keepReasoningOnlyAssistant(message llm.Message) bool {
	return message.Role == "assistant" && HasChatCompatibleAssistantPayload(message)
}

// substituteEmptyToolOutput (fork; moved from messages.go)
func substituteEmptyToolOutput(message llm.Message) (llm.Message, bool, bool) {
	if message.Role == "tool" {
		placeholder := emptyToolOutputPlaceholder
		message.Content = llm.MessageContent{Content: &placeholder}
		return message, true, true
	}
	return message, false, false
}

// visibleChatContentPart (fork; moved from messages.go)
func visibleChatContentPart(part llm.MessageContentPart) bool {
	switch part.Type {
	case "text", "input_text", "output_text":
		return part.Text != nil && strings.TrimSpace(*part.Text) != ""
	case "image_url":
		return part.ImageURL != nil && strings.TrimSpace(part.ImageURL.URL) != ""
	case "video_url":
		return part.VideoURL != nil && strings.TrimSpace(part.VideoURL.URL) != ""
	case "input_audio":
		return part.InputAudio != nil && strings.TrimSpace(part.InputAudio.Data) != ""
	case "compaction", "compaction_summary":
		// Chat conversion drops these parts, so they cannot keep a message alive.
		return false
	default:
		return true
	}
}
