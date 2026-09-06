package responses

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/looplj/axonhub/llm"
	"github.com/samber/lo"
)

// toolSearchOutputText (fork; moved from outbound_convert.go)
func toolSearchOutputText(content llm.MessageContent) string {
	if content.Content != nil {
		return *content.Content
	}
	var result strings.Builder
	for _, part := range content.MultipleContent {
		if part.Type == "text" && part.Text != nil {
			result.WriteString(*part.Text)
		}
	}
	return result.String()
}

// synchronizeToolSearchOutputMessages (fork; moved from outbound_convert.go)
func synchronizeToolSearchOutputMessages(
	messages []llm.Message,
	tools []llm.Tool,
	requestExt *llm.OpenAIResponsesRequestExtensions,
) ([]llm.Message, error) {
	if requestExt == nil || len(requestExt.RawInputItems) == 0 {
		return messages, nil
	}

	trackedOrigins := make(map[string][]string)
	for _, fragment := range requestExt.RawInputItems {
		if fragment.Type == "tool_search_output" && fragment.CallID != "" {
			trackedOrigins[fragment.CallID] = append(
				trackedOrigins[fragment.CallID], fmt.Sprintf("input:%d:", fragment.OriginalIndex),
			)
		}
	}
	if len(trackedOrigins) == 0 {
		return messages, nil
	}

	result := append([]llm.Message(nil), messages...)
	seenOutputs := make(map[string]int)
	toolSearchCallByID := make(map[string]bool)
	for messageIndex := range result {
		message := &result[messageIndex]
		if message.Role == "assistant" {
			for _, call := range message.ToolCalls {
				if call.ID != "" {
					toolSearchCallByID[call.ID] = call.ResponseToolSearchCall != nil
				}
			}
		}
		if message.Role != "tool" || message.ToolCallID == nil {
			continue
		}
		callID := *message.ToolCallID
		if !toolSearchCallByID[callID] {
			continue
		}
		origins := trackedOrigins[callID]
		occurrence := seenOutputs[callID]
		if occurrence >= len(origins) {
			continue
		}
		seenOutputs[callID] = occurrence + 1
		originPrefix := origins[occurrence]

		definitions := make([]Tool, 0)
		hasOriginTools := false
		for _, tool := range tools {
			if tool.ResponsesOrigin != "tool_search_output" || tool.ResponsesOriginCallID != callID ||
				!strings.HasPrefix(tool.ResponsesRawID, originPrefix) {
				continue
			}
			hasOriginTools = true
			definition, ok, err := responseToolSearchOutputDefinition(tool)
			if err != nil {
				return nil, err
			}
			if ok {
				definitions = append(definitions, definition)
			}
		}
		if len(definitions) == 0 && hasOriginTools {
			// Origin tools exist but none convert to definitions (for example
			// opaque declarations). Overwriting the content with an empty array
			// would erase the original output, so keep it and let
			// convertToolMessageWithType keep or decode the original value.
			continue
		}
		encoded, err := json.Marshal(definitions)
		if err != nil {
			return nil, err
		}
		message.Content = llm.MessageContent{Content: lo.ToPtr(string(encoded))}
	}

	return result, nil
}

// responseToolSearchOutputDefinition (fork; moved from outbound_convert.go)
func responseToolSearchOutputDefinition(src llm.Tool) (Tool, bool, error) {
	switch src.Type {
	case llm.ToolTypeFunction:
		return convertFunctionToTool(src), true, nil
	case llm.ToolTypeImageGeneration:
		return convertImageGenerationToTool(src), true, nil
	case llm.ToolTypeWebSearch, llm.ToolTypeGoogleSearch:
		return convertWebSearchToTool(src), true, nil
	case llm.ToolTypeResponsesCustomTool:
		return convertCustomToTool(src), true, nil
	case llm.ToolTypeResponsesToolSearch:
		if src.ResponseToolSearch == nil {
			return Tool{}, false, nil
		}
		parameters := map[string]any{}
		if len(src.ResponseToolSearch.Parameters) > 0 {
			if err := json.Unmarshal(src.ResponseToolSearch.Parameters, &parameters); err != nil {
				return Tool{}, false, fmt.Errorf("failed to decode tool search parameters: %w", err)
			}
		}
		return Tool{
			Type: "tool_search", Execution: src.ResponseToolSearch.Execution,
			Description: src.ResponseToolSearch.Description, Parameters: parameters,
		}, true, nil
	default:
		return Tool{}, false, nil
	}
}

// toolSearchMissingArguments (fork; moved from outbound_stream.go)
func toolSearchMissingArguments(callID, forwardedArgs, finalArgs string) (string, error) {
	switch {
	case forwardedArgs == "":
		return finalArgs, nil
	case strings.HasPrefix(finalArgs, forwardedArgs):
		return strings.TrimPrefix(finalArgs, forwardedArgs), nil
	case equalJSONValues(forwardedArgs, finalArgs):
		return "", nil
	default:
		return "", fmt.Errorf("tool search call arguments mismatch for call_id %q", callID)
	}
}

// equalDecodedJSONValues (fork; moved from outbound_stream.go)
func equalDecodedJSONValues(left, right any) bool {
	switch leftValue := left.(type) {
	case json.Number:
		rightValue, ok := right.(json.Number)
		if !ok {
			return false
		}
		if leftValue.String() == rightValue.String() {
			return true
		}
		var leftRat, rightRat big.Rat
		_, leftOK := leftRat.SetString(leftValue.String())
		_, rightOK := rightRat.SetString(rightValue.String())
		return leftOK && rightOK && leftRat.Cmp(&rightRat) == 0
	case []any:
		rightValue, ok := right.([]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for index := range leftValue {
			if !equalDecodedJSONValues(leftValue[index], rightValue[index]) {
				return false
			}
		}
		return true
	case map[string]any:
		rightValue, ok := right.(map[string]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for key, leftItem := range leftValue {
			rightItem, exists := rightValue[key]
			if !exists || !equalDecodedJSONValues(leftItem, rightItem) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(left, right)
	}
}
