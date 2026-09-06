package responses

import (
	"encoding/json"
	"fmt"

	"github.com/samber/lo"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
)

// validateUniqueTopLevelNamespaces (moved from inbound.go)
func validateUniqueTopLevelNamespaces(tools []Tool) error {
	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool.Type != "namespace" || tool.Name == "" {
			continue
		}
		if _, exists := seen[tool.Name]; exists {
			return fmt.Errorf(
				"%w: duplicate_namespace: namespace %q appears in multiple tool declarations",
				transformer.ErrInvalidRequest, tool.Name,
			)
		}
		seen[tool.Name] = struct{}{}
	}
	return nil
}

// isToolCallItemType (moved from inbound.go)
func isToolCallItemType(itemType string) bool {
	switch itemType {
	case "function_call", "custom_tool_call", "tool_search_call":
		return true
	default:
		return false
	}
}

// isToolOutputItemType (moved from inbound.go)
func isToolOutputItemType(itemType string) bool {
	switch itemType {
	case "function_call_output", "custom_tool_call_output", "tool_search_output":
		return true
	default:
		return false
	}
}

// mergeToolOutputItems (moved from inbound.go)
func mergeToolOutputItems(items []Item, startIdx int) ([]*llm.Message, int, error) {
	if startIdx >= len(items) {
		return nil, 0, nil
	}

	type toolOutputAccumulator struct {
		parts    []llm.MessageContentPart
		toolName string
	}

	order := make([]string, 0)
	byCallID := make(map[string]*toolOutputAccumulator)
	consumed := 0
	for idx := startIdx; idx < len(items); idx++ {
		it := &items[idx]
		if !isToolOutputItemType(it.Type) {
			break
		}
		outMsg, err := convertItemToMessage(it)
		if err != nil {
			return nil, consumed, err
		}
		consumed++
		if outMsg != nil {
			acc, ok := byCallID[it.CallID]
			if !ok {
				acc = &toolOutputAccumulator{}
				byCallID[it.CallID] = acc
				order = append(order, it.CallID)
			}
			if outMsg.ToolCallName != nil && acc.toolName == "" {
				acc.toolName = *outMsg.ToolCallName
			}
			acc.parts = appendMessageContentParts(acc.parts, outMsg.Content)
		}
	}
	if consumed == 0 {
		return nil, 0, nil
	}

	messages := make([]*llm.Message, 0, len(order))
	for _, callID := range order {
		acc := byCallID[callID]
		messages = append(messages, toolOutputMessage(callID, acc.toolName, acc.parts))
	}
	return messages, consumed, nil
}

// toolOutputMessage (moved from inbound.go)
func toolOutputMessage(callID, toolName string, parts []llm.MessageContentPart) *llm.Message {
	msg := &llm.Message{
		Role:       "tool",
		ToolCallID: lo.ToPtr(callID),
	}
	if toolName != "" {
		msg.ToolCallName = lo.ToPtr(toolName)
	}
	switch len(parts) {
	case 0:
		// All outputs were empty; emit an empty tool message so the tool_call_id
		// still has a corresponding tool response for Chat providers.
		empty := ""
		msg.Content = llm.MessageContent{Content: &empty}
	case 1:
		if parts[0].Type == "text" && parts[0].Text != nil {
			msg.Content = llm.MessageContent{Content: parts[0].Text}
		} else {
			msg.Content = llm.MessageContent{MultipleContent: parts}
		}
	default:
		msg.Content = llm.MessageContent{MultipleContent: parts}
	}
	return msg
}

// appendMessageContentParts (moved from inbound.go)
func appendMessageContentParts(dst []llm.MessageContentPart, content llm.MessageContent) []llm.MessageContentPart {
	if len(content.MultipleContent) > 0 {
		return append(dst, content.MultipleContent...)
	}
	if content.Content != nil && *content.Content != "" {
		return append(dst, llm.MessageContentPart{
			Type: "text",
			Text: content.Content,
		})
	}
	return dst
}

// convertToolsToLLMWithRawIDs (moved from inbound.go)
func convertToolsToLLMWithRawIDs(tools []Tool, prefix string) ([]llm.Tool, error) {
	result := make([]llm.Tool, 0, len(tools))
	for declarationIndex, tool := range tools {
		converted, err := convertToolsToLLM([]Tool{tool})
		if err != nil {
			return nil, err
		}
		needsRawID := prefix != "tools" || !isStructurallyRepresentedToolType(tool.Type) || tool.Type == "tool_search"
		if needsRawID {
			rawID := fmt.Sprintf("%s:%d", prefix, declarationIndex)
			for i := range converted {
				converted[i].ResponsesRawID = rawID
			}
		}
		result = append(result, converted...)
	}
	return result, nil
}

// convertToolDeclaration (moved from inbound.go)
func convertToolDeclaration(tool Tool, namespace string) ([]llm.Tool, error) {
	if namespace != "" && !namespaceCallableToolType(tool) {
		return []llm.Tool{opaqueResponsesTool(tool, "raw_tool", namespace)}, nil
	}

	switch tool.Type {
	case "function":
		params, err := json.Marshal(tool.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal function parameters: %w", err)
		}
		function := llm.Function{
			Name:         tool.Name,
			Description:  tool.Description,
			Parameters:   params,
			Strict:       tool.Strict,
			DeferLoading: tool.DeferLoading,
		}
		if namespace != "" {
			function.Name = llm.JoinNamespaceFunctionName(namespace, tool.Name)
			function.Namespace = namespace
		}
		return []llm.Tool{{Type: "function", Function: function}}, nil

	case "image_generation":
		return []llm.Tool{{
			Type: llm.ToolTypeImageGeneration,
			ImageGeneration: &llm.ImageGeneration{
				Background:        tool.Background,
				InputFidelity:     tool.InputFidelity,
				Moderation:        tool.Moderation,
				OutputCompression: tool.OutputCompression,
				OutputFormat:      tool.OutputFormat,
				PartialImages:     tool.PartialImages,
				Quality:           tool.Quality,
				Size:              tool.Size,
			},
		}}, nil

	case "web_search":
		webSearch := &llm.WebSearch{}
		if tool.Filters != nil {
			webSearch.AllowedDomains = append(webSearch.AllowedDomains, tool.Filters.AllowedDomains...)
		}
		if tool.UserLocation != nil {
			locationType := tool.UserLocation.Type
			if locationType == "" {
				locationType = "approximate"
			}
			webSearch.UserLocation = llm.WebSearchToolUserLocation{
				Type:     locationType,
				City:     tool.UserLocation.City,
				Country:  tool.UserLocation.Country,
				Region:   tool.UserLocation.Region,
				Timezone: tool.UserLocation.Timezone,
			}
		}
		return []llm.Tool{{Type: llm.ToolTypeWebSearch, WebSearch: webSearch}}, nil

	case "custom":
		return []llm.Tool{customToolFromDeclaration(tool, namespace)}, nil

	case "tool_search":
		params, err := json.Marshal(tool.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool search parameters: %w", err)
		}
		return []llm.Tool{{
			Type: llm.ToolTypeResponsesToolSearch,
			ResponseToolSearch: &llm.ResponseToolSearch{
				Execution:   tool.Execution,
				Description: tool.Description,
				Parameters:  params,
			},
		}}, nil

	case "namespace":
		result := make([]llm.Tool, 0, len(tool.Tools))
		for _, subTool := range tool.Tools {
			converted, err := convertToolDeclaration(subTool, tool.Name)
			if err != nil {
				return nil, err
			}
			for i := range converted {
				converted[i].ResponsesNamespaceDescription = tool.Description
			}
			result = append(result, converted...)
		}
		return result, nil

	default:
		if !isGenericClientFunctionLike(tool) {
			return []llm.Tool{opaqueResponsesTool(tool, "raw_tool", namespace)}, nil
		}
		params, err := json.Marshal(tool.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal %s tool parameters: %w", tool.Type, err)
		}
		converted := llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name: tool.Name, Description: tool.Description,
				Parameters: params, Strict: tool.Strict, DeferLoading: tool.DeferLoading,
			},
			ResponsesOrigin:     "raw_tool",
			ResponsesSourceType: tool.Type,
		}
		if namespace != "" {
			converted.Function.Name = llm.JoinNamespaceFunctionName(namespace, tool.Name)
			converted.Function.Namespace = namespace
		}
		return []llm.Tool{converted}, nil
	}
}

// isGenericClientFunctionLike (moved from inbound.go)
func isGenericClientFunctionLike(tool Tool) bool {
	return isFunctionLike(tool) && tool.Execution == "client"
}

// namespaceCallableToolType (moved from inbound.go)
func namespaceCallableToolType(tool Tool) bool {
	switch tool.Type {
	case "function", "custom":
		return true
	default:
		return isGenericClientFunctionLike(tool)
	}
}

// isFunctionLike (moved from inbound.go)
func isFunctionLike(tool Tool) bool {
	return tool.Name != "" && tool.Parameters != nil
}

// customToolFromDeclaration (moved from inbound.go)
func customToolFromDeclaration(tool Tool, namespace string) llm.Tool {
	customTool := &llm.ResponseCustomTool{
		Name:        tool.Name,
		Namespace:   namespace,
		Description: tool.Description,
	}
	if tool.Format != nil {
		customTool.Format = &llm.ResponseCustomToolFormat{
			Type:       tool.Format.Type,
			Syntax:     tool.Format.Syntax,
			Definition: tool.Format.Definition,
		}
	}
	return llm.Tool{
		Type:               llm.ToolTypeResponsesCustomTool,
		ResponseCustomTool: customTool,
	}
}

// opaqueResponsesTool (moved from inbound.go)
func opaqueResponsesTool(tool Tool, origin, namespace string) llm.Tool {
	return llm.Tool{
		Type: llm.ToolTypeResponsesOpaqueTool,
		ResponseOpaqueTool: &llm.ResponseOpaqueTool{
			SourceType: tool.Type, Name: tool.Name, Namespace: namespace, Execution: tool.Execution,
			Description: tool.Description,
		},
		ResponsesOrigin: origin,
	}
}

// responsesToolCallKind (fork; moved from inbound_stream.go)
func responsesToolCallKind(call *llm.ToolCall) string {
	switch {
	case call.ResponseToolSearchCall != nil:
		return "tool_search"
	case call.ResponseCustomToolCall != nil:
		return "custom"
	default:
		return "function"
	}
}

// rememberNamespaceMember (fork; moved from outbound.go)
func rememberNamespaceMember(namespace, description string, member Tool, members map[string][]Tool, tools *[]Tool) {
	if _, exists := members[namespace]; !exists {
		*tools = append(*tools, Tool{
			Type: "namespace", Name: namespace, Description: description,
		})
	}
	members[namespace] = append(members[namespace], member)
}

// validateUniqueNamespaceGroups (fork; moved from outbound.go)
func validateUniqueNamespaceGroups(
	tools []llm.Tool,
	replayRawInput bool,
	requestExt *llm.OpenAIResponsesRequestExtensions,
) error {
	rawSeen := make(map[string]struct{})
	descriptions := make(map[string]string)
	if requestExt != nil {
		for _, fragment := range requestExt.RawTools {
			if len(fragment.Raw) == 0 {
				continue
			}
			var rawTool Tool
			if json.Unmarshal(fragment.Raw, &rawTool) != nil || rawTool.Type != "namespace" || rawTool.Name == "" {
				continue
			}
			if _, exists := rawSeen[rawTool.Name]; exists {
				return fmt.Errorf(
					"%w: duplicate_namespace: namespace %q appears in multiple tool declarations",
					transformer.ErrInvalidRequest, rawTool.Name,
				)
			}
			rawSeen[rawTool.Name] = struct{}{}
		}
	}
	for _, tool := range tools {
		if !responsesOriginToolEmitsTopLevel(tool, replayRawInput) {
			continue
		}
		namespace := namespaceOfResponsesTool(tool)
		if namespace == "" {
			continue
		}
		description := tool.ResponsesNamespaceDescription
		previous, exists := descriptions[namespace]
		if !exists {
			descriptions[namespace] = description
		} else if previous != description {
			return fmt.Errorf(
				"%w: namespace_description_conflict: namespace %q has multiple descriptions",
				transformer.ErrInvalidRequest, namespace,
			)
		}
	}
	return nil
}
