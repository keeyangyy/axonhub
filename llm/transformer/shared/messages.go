package shared

import (
	"github.com/looplj/axonhub/llm"
)

// FilterOutResponseCustomToolMessages removes Responses-only custom tool calls
// from assistant messages and drops tool result messages that correspond to
// those removed custom tool calls.
//
// This is intended for compatibility when a request originates from an OpenAI
// Responses session and is then routed to a non-Responses channel. In that
// case, Responses-only custom tools must be stripped from the message history
// before the outbound transformer encodes the request for the target channel.
func FilterOutResponseCustomToolMessages(messages []llm.Message) []llm.Message {
	return filterOutToolLifecycleMessages(messages, func(toolCall llm.ToolCall) bool {
		return toolCall.Type == llm.ToolTypeResponsesCustomTool || toolCall.ResponseCustomToolCall != nil
	})
}

// FilterOutResponsesChatToolLifecycleMessages removes calls that need the
// reversible Responses-to-Chat adapter and their paired tool outputs. Plain
// function calls remain available to provider-specific Chat transformers.

// HasChatCompatibleAssistantPayload reports whether an assistant message still
// contains substantive data after Responses-only lifecycle fields are removed.
// Other roles have different validation rules and always pass through.

// SanitizeChatToolArguments repairs assistant tool-call arguments that are not
// valid JSON. Truncated streams can leave clients replaying partial arguments,
// which strict Chat providers reject for the whole request. Returns the
// repaired slice and whether anything changed.

// repairToolCallArguments substitutes an empty JSON object for arguments that
// no Chat provider can accept, reporting whether the call changed.

const emptyToolOutputPlaceholder = "(empty)"

// SanitizeChatMessageContent removes messages that carry no expressible text
// content and substitutes empty tool outputs, so strict Chat providers do not
// reject replayed history with "text content is empty". Interrupted upstream
// turns leave empty output items in client history; replaying them verbatim
// poisons every subsequent request.

// keepReasoningOnlyAssistant retains assistant turns whose only substantive
// payload is reasoning, refusal, or audio. HasChatCompatibleAssistantPayload
// counts these as valid payloads; dropping the message would make replayed
// history disagree with that rule and lose echoable reasoning summaries.

// substituteEmptyToolOutput keeps tool messages paired with their call by
// giving them a placeholder, and drops every other contentless message.
