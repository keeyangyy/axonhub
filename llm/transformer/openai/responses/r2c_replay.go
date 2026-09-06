package responses

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
)

// representedRawToolCount (moved from request_extensions.go)
func representedRawToolCount(tool Tool) int {
	if tool.Type == ToolTypeToolSearch {
		return 1
	}
	if tool.Type != ToolTypeNamespace {
		return 0
	}

	count := 0
	for _, subTool := range tool.Tools {
		if namespaceCallableToolType(subTool) {
			count++
		}
	}

	return count
}

// rawToolGroup (moved from request_extensions.go)
type rawToolGroup struct {
	fragment   llm.OpenAIResponsesRawFragment
	namespace  string
	lossy      bool
	signatures []string
}

// rawReplayPlan (moved from request_extensions.go)
type rawReplayPlan struct {
	steps    []json.RawMessage
	replayed bool
}

// buildRawToolGroups (moved from request_extensions.go)
func buildRawToolGroups(fragments []llm.OpenAIResponsesRawFragment) []rawToolGroup {
	groups := make([]rawToolGroup, 0, len(fragments))
	for _, fragment := range fragments {
		if len(fragment.Raw) == 0 {
			continue
		}
		var rawTool Tool
		if json.Unmarshal(fragment.Raw, &rawTool) != nil {
			continue
		}
		converted, err := convertToolsToLLM([]Tool{rawTool})
		if err != nil || len(converted) == 0 {
			continue
		}
		for i := range converted {
			converted[i].ResponsesRawID = fmt.Sprintf("tools:%d", fragment.OriginalIndex)
		}
		namespace := ""
		lossy := false
		if rawTool.Type == ToolTypeNamespace {
			namespace = rawTool.Name
			for _, subTool := range rawTool.Tools {
				if !namespaceCallableToolType(subTool) {
					lossy = true
					break
				}
			}
		}
		groups = append(groups, rawToolGroup{
			fragment: fragment, namespace: namespace, lossy: lossy, signatures: replayToolSignatures(converted),
		})
	}
	return groups
}

// buildReplayPlan (moved from request_extensions.go)
func buildReplayPlan(
	currentTools []llm.Tool,
	groups []rawToolGroup,
	structuredTools []json.RawMessage,
	replayRawInput bool,
) (*rawReplayPlan, error) {
	lossyNamespaces := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if group.lossy {
			lossyNamespaces[group.namespace] = struct{}{}
		}
	}
	currentUnsupportedNamespaces := make(map[string]struct{})
	for _, tool := range currentTools {
		if tool.Type == llm.ToolTypeResponsesOpaqueTool && tool.ResponseOpaqueTool != nil &&
			tool.ResponseOpaqueTool.Namespace != "" {
			currentUnsupportedNamespaces[tool.ResponseOpaqueTool.Namespace] = struct{}{}
		}
	}

	currentSignatures := replayToolSignatures(currentTools)
	plan := &rawReplayPlan{steps: make([]json.RawMessage, 0, len(structuredTools)+len(groups))}
	usedGroups := make([]bool, len(groups))
	structuredIndex := 0
	activeNamespace := ""
	for toolIndex := 0; toolIndex < len(currentTools); {
		matchedGroup := -1
		for groupIndex := range groups {
			group := &groups[groupIndex]
			end := toolIndex + len(group.signatures)
			if usedGroups[groupIndex] || end > len(currentSignatures) ||
				!slices.Equal(currentSignatures[toolIndex:end], group.signatures) {
				continue
			}
			// A raw namespace can match only the complete current group. A
			// prefix match means an intermediate transform added a member;
			// replaying the old fragment would silently drop that addition.
			if group.namespace != "" && end < len(currentTools) &&
				namespaceOfResponsesTool(currentTools[end]) == group.namespace {
				continue
			}
			matchedGroup = groupIndex
			break
		}

		if matchedGroup >= 0 {
			group := groups[matchedGroup]
			usedGroups[matchedGroup] = true
			plan.steps = append(plan.steps, group.fragment.Raw)
			if group.namespace != "" {
				if namespaceGroupEmitsStructured(currentTools[toolIndex:toolIndex+len(group.signatures)], replayRawInput) &&
					group.namespace != activeNamespace {
					if !namespaceWrapperMatches(structuredTools, structuredIndex, group.namespace) {
						return nil, nil
					}
					structuredIndex++
					activeNamespace = group.namespace
				}
			} else {
				for i := range len(group.signatures) {
					if !responsesToolEmitsStructured(currentTools[toolIndex+i], replayRawInput) {
						continue
					}
					if structuredIndex >= len(structuredTools) {
						return nil, nil
					}
					structuredIndex++
				}
				activeNamespace = ""
			}
			if structuredIndex > len(structuredTools) {
				return nil, nil
			}
			toolIndex += len(group.signatures)
			plan.replayed = true
			continue
		}

		if namespace := namespaceOfResponsesTool(currentTools[toolIndex]); namespace != "" {
			_, lossy := lossyNamespaces[namespace]
			_, stillUnsupported := currentUnsupportedNamespaces[namespace]
			if stillUnsupported &&
				(lossy || namespaceGroupModified(groups, currentTools, currentSignatures, toolIndex, namespace)) {
				return nil, fmt.Errorf(
					"%w: unsupported_namespace_replay: namespace %q was modified and contains member type(s) without a structural Responses codec",
					transformer.ErrInvalidRequest,
					namespace,
				)
			}
		}

		if responsesToolEmitsStructured(currentTools[toolIndex], replayRawInput) {
			if namespace := namespaceOfResponsesTool(currentTools[toolIndex]); namespace != "" {
				if namespace == activeNamespace {
					toolIndex++
					continue
				}
				if !namespaceWrapperMatches(structuredTools, structuredIndex, namespace) {
					return nil, nil
				}
				plan.steps = append(plan.steps, structuredTools[structuredIndex])
				structuredIndex++
				activeNamespace = namespace
			} else {
				if structuredIndex >= len(structuredTools) {
					return nil, nil
				}
				plan.steps = append(plan.steps, structuredTools[structuredIndex])
				structuredIndex++
				activeNamespace = ""
			}
		}
		toolIndex++
	}

	if structuredIndex != len(structuredTools) {
		return nil, nil
	}

	return plan, nil
}

// namespaceGroupModified (moved from request_extensions.go)
func namespaceGroupModified(
	groups []rawToolGroup,
	currentTools []llm.Tool,
	currentSignatures []string,
	toolIndex int,
	namespace string,
) bool {
	for i := range groups {
		if groups[i].namespace != namespace {
			continue
		}
		end := toolIndex + len(groups[i].signatures)
		complete := end <= len(currentSignatures) &&
			slices.Equal(currentSignatures[toolIndex:end], groups[i].signatures) &&
			(end == len(currentTools) || namespaceOfResponsesTool(currentTools[end]) != namespace)
		return !complete
	}
	return false
}

// namespaceGroupEmitsStructured (moved from request_extensions.go)
func namespaceGroupEmitsStructured(tools []llm.Tool, replayRawInput bool) bool {
	for _, tool := range tools {
		if responsesToolEmitsStructured(tool, replayRawInput) {
			return true
		}
	}
	return false
}

// namespaceWrapperMatches (moved from request_extensions.go)
func namespaceWrapperMatches(
	structuredTools []json.RawMessage,
	structuredIndex int,
	namespace string,
) bool {
	if structuredIndex >= len(structuredTools) {
		return false
	}
	var structuredTool Tool
	if json.Unmarshal(structuredTools[structuredIndex], &structuredTool) != nil ||
		structuredTool.Type != ToolTypeNamespace || structuredTool.Name != namespace {
		return false
	}
	return true
}

// namespaceOfResponsesTool (moved from request_extensions.go)
func namespaceOfResponsesTool(tool llm.Tool) string {
	switch {
	case tool.Type == llm.ToolTypeFunction && tool.Function.Namespace != "":
		return tool.Function.Namespace
	case tool.Type == llm.ToolTypeResponsesCustomTool && tool.ResponseCustomTool != nil:
		return tool.ResponseCustomTool.Namespace
	case tool.Type == llm.ToolTypeResponsesOpaqueTool && tool.ResponseOpaqueTool != nil:
		return tool.ResponseOpaqueTool.Namespace
	default:
		return ""
	}
}

// responsesOriginToolEmitsTopLevel (moved from request_extensions.go)
func responsesOriginToolEmitsTopLevel(tool llm.Tool, replayRawInput bool) bool {
	switch tool.ResponsesOrigin {
	case "":
		return true
	case "raw_tool":
		return true
	case "additional_tools":
		return !replayRawInput
	case "tool_search_output":
		return false
	default:
		return false
	}
}

// responsesToolEmitsStructured (moved from request_extensions.go)
func responsesToolEmitsStructured(tool llm.Tool, replayRawInput bool) bool {
	if !responsesOriginToolEmitsTopLevel(tool, replayRawInput) {
		return false
	}
	switch tool.Type {
	case llm.ToolTypeFunction, llm.ToolTypeImageGeneration, llm.ToolTypeWebSearch, llm.ToolTypeGoogleSearch,
		llm.ToolTypeResponsesCustomTool, llm.ToolTypeResponsesToolSearch:
		return true
	default:
		return false
	}
}

// rawInputReplayMatchesCurrent (moved from request_extensions.go)
func rawInputReplayMatchesCurrent(
	requestExt *llm.OpenAIResponsesRequestExtensions,
	currentMessages []llm.Message,
	currentTools []llm.Tool,
) bool {
	return requestExt != nil && len(requestExt.RawInputItems) > 0 &&
		slices.Equal(requestExt.RawInputMessages, replayMessageSignatures(currentMessages)) &&
		slices.Equal(requestExt.RawInputTools, replayInputToolSignatures(currentTools))
}

// replayToolSignatures (moved from request_extensions.go)
func replayToolSignatures(tools []llm.Tool) []string {
	signatures := make([]string, len(tools))
	for i := range tools {
		signatures[i] = replayToolSignature(tools[i])
	}
	return signatures
}

// replayToolSignature (moved from request_extensions.go)
func replayToolSignature(tool llm.Tool) string {
	data, err := json.Marshal(struct {
		Tool                 llm.Tool `json:"tool"`
		Origin               string   `json:"origin,omitempty"`
		SourceType           string   `json:"source_type,omitempty"`
		RawID                string   `json:"raw_id,omitempty"`
		OriginCallID         string   `json:"origin_call_id,omitempty"`
		NamespaceDescription string   `json:"namespace_description,omitempty"`
	}{
		Tool: tool, Origin: tool.ResponsesOrigin, SourceType: tool.ResponsesSourceType,
		RawID: tool.ResponsesRawID, OriginCallID: tool.ResponsesOriginCallID,
		NamespaceDescription: tool.ResponsesNamespaceDescription,
	})
	if err != nil {
		return "\x00invalid"
	}
	return string(data)
}

// replayMessageSignatures (moved from request_extensions.go)
func replayMessageSignatures(messages []llm.Message) []string {
	signatures := make([]string, len(messages))
	for i := range messages {
		data, err := json.Marshal(messages[i])
		if err != nil {
			signatures[i] = "\x00invalid"
			continue
		}
		signatures[i] = string(data)
	}
	return signatures
}

// replayInputToolSignatures (moved from request_extensions.go)
func replayInputToolSignatures(tools []llm.Tool) []string {
	signatures := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.ResponsesOrigin == "additional_tools" || tool.ResponsesOrigin == "tool_search_output" {
			signatures = append(signatures, replayToolSignature(tool))
		}
	}
	return signatures
}
