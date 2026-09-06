package responses

import (
	"encoding/json"

	"github.com/samber/lo"

	"github.com/looplj/axonhub/llm"
)

var rawCreateRequestFields = []string{
	"context_management",
	"conversation",
	"moderation",
	"prompt",
	"prompt_cache_options",
}

var rawCompactRequestFields = []string{
	"previous_response_id",
	"prompt_cache_options",
	"prompt_cache_retention",
	"service_tier",
}

func attachOpenAIResponsesRequestExtensions(chatReq *llm.Request, req *Request, rawBody []byte) {
	if chatReq == nil || req == nil {
		return
	}

	raw := parseRawRequestFragments(rawBody)
	reasoningContext := ""
	if req.Reasoning != nil {
		reasoningContext = req.Reasoning.Context
	}
	requestExt := &llm.OpenAIResponsesRequestExtensions{
		ReasoningContext: reasoningContext,
		RawFields:        selectRawRequestFields(raw.Fields, rawCreateRequestFields),
		RawTools:         buildRawOnlyToolFragments(req.Tools, raw.Tools),
		RawToolChoice:    rawUnsupportedToolChoice(req.ToolChoice, raw.ToolChoice),
		RawInputItems:    buildRawOnlyInputFragments(req.Input, raw.InputItems),
		RawInputMessages: replayMessageSignatures(chatReq.Messages),
		RawInputTools:    replayInputToolSignatures(chatReq.Tools),
	}

	if requestExt.ReasoningContext == "" && len(requestExt.RawFields) == 0 && len(requestExt.RawTools) == 0 && len(requestExt.RawToolChoice) == 0 && len(requestExt.RawInputItems) == 0 {
		return
	}

	ext := llm.EnsureOpenAIResponsesProviderExtensions(chatReq)
	if ext == nil {
		return
	}
	ext.Request = requestExt
}

type rawRequestFragments struct {
	Fields     map[string]json.RawMessage
	Tools      []json.RawMessage
	ToolChoice json.RawMessage
	InputItems []json.RawMessage
}

func parseRawRequestFragments(rawBody []byte) rawRequestFragments {
	if len(rawBody) == 0 {
		return rawRequestFragments{}
	}

	var raw struct {
		Tools      []json.RawMessage `json:"tools"`
		ToolChoice json.RawMessage   `json:"tool_choice"`
		Input      json.RawMessage   `json:"input"`
	}
	if err := json.Unmarshal(rawBody, &raw); err != nil {
		return rawRequestFragments{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &fields); err != nil {
		return rawRequestFragments{}
	}

	var inputItems []json.RawMessage
	if len(raw.Input) > 0 && json.Unmarshal(raw.Input, &inputItems) != nil {
		inputItems = nil
	}

	return rawRequestFragments{
		Fields:     fields,
		Tools:      raw.Tools,
		ToolChoice: raw.ToolChoice,
		InputItems: inputItems,
	}
}

func attachOpenAIResponsesRawRequestFields(chatReq *llm.Request, rawBody []byte, fieldNames []string) {
	if chatReq == nil || len(rawBody) == 0 {
		return
	}

	var fields map[string]json.RawMessage
	if json.Unmarshal(rawBody, &fields) != nil {
		return
	}
	rawFields := selectRawRequestFields(fields, fieldNames)
	if len(rawFields) == 0 {
		return
	}

	ext := llm.EnsureOpenAIResponsesProviderExtensions(chatReq)
	if ext == nil {
		return
	}
	if ext.Request == nil {
		ext.Request = &llm.OpenAIResponsesRequestExtensions{}
	}
	if ext.Request.RawFields == nil {
		ext.Request.RawFields = make(map[string]json.RawMessage, len(rawFields))
	}
	for key, value := range rawFields {
		ext.Request.RawFields[key] = value
	}
}

func selectRawRequestFields(fields map[string]json.RawMessage, fieldNames []string) map[string]json.RawMessage {
	if len(fields) == 0 || len(fieldNames) == 0 {
		return nil
	}

	selected := make(map[string]json.RawMessage, len(fieldNames))
	for _, name := range fieldNames {
		if value, ok := fields[name]; ok {
			selected[name] = cloneRaw(value)
		}
	}
	if len(selected) == 0 {
		return nil
	}

	return selected
}
func buildRawOnlyToolFragments(tools []Tool, rawTools []json.RawMessage) []llm.OpenAIResponsesRawFragment {
	if len(tools) == 0 {
		return nil
	}

	fragments := make([]llm.OpenAIResponsesRawFragment, 0, len(tools))
	for i := range tools {
		if i >= len(rawTools) || len(rawTools[i]) == 0 || (isStructurallyRepresentedToolType(tools[i].Type) && tools[i].Type != ToolTypeToolSearch) {
			continue
		}

		fragments = append(fragments, llm.OpenAIResponsesRawFragment{
			Type:                 tools[i].Type,
			Name:                 tools[i].Name,
			OriginalIndex:        i,
			RepresentedToolCount: representedRawToolCount(tools[i]),
			Raw:                  cloneRaw(rawTools[i]),
		})
	}

	return fragments
}

// representedRawToolCount counts tool declarations represented by one preserved raw item.

func isStructurallyRepresentedToolType(toolType string) bool {
	switch toolType {
	case llm.ToolTypeFunction, "image_generation", "web_search", ToolTypeCustom, ToolTypeToolSearch:
		return true
	default:
		return false
	}
}

func responseToolSignature(tool Tool) string {
	switch tool.Type {
	case llm.ToolTypeFunction, ToolTypeCustom:
		return tool.Type + ":" + tool.Name
	default:
		return tool.Type
	}
}

func rawUnsupportedToolChoice(choice *ToolChoice, rawChoice json.RawMessage) json.RawMessage {
	if choice == nil || len(rawChoice) == 0 {
		return nil
	}

	classification := ClassifyRawToolChoice(rawChoice)
	if classification.FullyRepresented {
		var decodedRaw ToolChoice
		if err := json.Unmarshal(rawChoice, &decodedRaw); err != nil {
			return cloneRaw(rawChoice)
		}
		if toolChoiceSignature(&decodedRaw) == toolChoiceSignature(choice) {
			return nil
		}
	}

	return cloneRaw(rawChoice)
}

func buildRawOnlyInputFragments(input Input, rawItems []json.RawMessage) []llm.OpenAIResponsesRawFragment {
	if len(input.Items) == 0 {
		return nil
	}

	fragments := make([]llm.OpenAIResponsesRawFragment, 0)
	for i := range input.Items {
		item := input.Items[i]
		preserveRepresented := item.Type == "tool_search_call" || item.Type == "tool_search_output" || item.Type == "agent_message"
		if i >= len(rawItems) || len(rawItems[i]) == 0 || (isStructurallyRepresentedInputItem(item.Type) && !preserveRepresented) {
			continue
		}

		fragments = append(fragments, llm.OpenAIResponsesRawFragment{
			Type:                 item.Type,
			Name:                 item.Name,
			CallID:               item.CallID,
			OriginalIndex:        i,
			RepresentedToolCount: lo.Ternary(preserveRepresented, 1, 0),
			Raw:                  cloneRaw(rawItems[i]),
		})
	}

	return fragments
}

func isStructurallyRepresentedInputItem(itemType string) bool {
	switch itemType {
	case "", "message", "input_text", "input_image", "function_call", "function_call_output",
		"custom_tool_call", "custom_tool_call_output", "tool_search_call", "tool_search_output",
		"reasoning", "compaction", "compaction_summary", "agent_message":
		return true
	default:
		return false
	}
}

func openAIResponsesRequestExtensions(llmReq *llm.Request) *llm.OpenAIResponsesRequestExtensions {
	if llmReq == nil || llmReq.ProviderExtensions == nil || llmReq.ProviderExtensions.OpenAIResponses == nil {
		return nil
	}
	requestExt := llmReq.ProviderExtensions.OpenAIResponses.Request

	return requestExt
}

func marshalRequestPayload(payload Request, llmReq *llm.Request) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	requestExt := openAIResponsesRequestExtensions(llmReq)
	if requestExt == nil {
		return body, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, err
	}
	mergeRawRequestFields(obj, requestExt)

	// Compute once: replay matching re-marshals every message and tool, so
	// both raw merges share the same verdict.
	replayRawInput := rawInputReplayMatchesCurrent(requestExt, llmReq.Messages, llmReq.Tools)

	tools, replayed, err := mergeRawOnlyTools(obj["tools"], requestExt, llmReq.Tools, replayRawInput)
	if err != nil {
		return nil, err
	}
	if replayed {
		toolsRaw, err := json.Marshal(tools)
		if err != nil {
			return nil, err
		}
		obj["tools"] = toolsRaw
	}

	if len(requestExt.RawToolChoice) > 0 && rawToolChoiceMatchesCurrentTools(requestExt.RawToolChoice, payload.ToolChoice) {
		obj["tool_choice"] = cloneRaw(requestExt.RawToolChoice)
	}

	if input, ok := mergeRawOnlyInputItems(obj["input"], requestExt, replayRawInput); ok {
		inputRaw, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		obj["input"] = inputRaw
	}

	return json.Marshal(obj)
}

func mergeRawRequestFields(obj map[string]json.RawMessage, requestExt *llm.OpenAIResponsesRequestExtensions) {
	if obj == nil || requestExt == nil {
		return
	}

	for key, value := range requestExt.RawFields {
		if _, exists := obj[key]; !exists && len(value) > 0 {
			obj[key] = cloneRaw(value)
		}
	}
}

func marshalCompactRequestPayload(payload CompactAPIRequest, llmReq *llm.Request) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	requestExt := openAIResponsesRequestExtensions(llmReq)
	if requestExt == nil || len(requestExt.RawFields) == 0 {
		return body, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, err
	}
	mergeRawRequestFields(obj, requestExt)

	return json.Marshal(obj)
}

func mergeRawOnlyInputItems(
	structuredRaw json.RawMessage,
	requestExt *llm.OpenAIResponsesRequestExtensions,
	replayRawInput bool,
) ([]json.RawMessage, bool) {
	if requestExt == nil || len(requestExt.RawInputItems) == 0 {
		return nil, false
	}
	if !replayRawInput {
		return nil, false
	}
	rawFragments := requestExt.RawInputItems

	var structuredItems []json.RawMessage
	if len(structuredRaw) > 0 {
		if err := json.Unmarshal(structuredRaw, &structuredItems); err != nil {
			return nil, false
		}
	}

	representedCount := 0
	for _, fragment := range rawFragments {
		representedCount += fragment.RepresentedToolCount
	}
	if representedCount > len(structuredItems) {
		return nil, false
	}
	total := len(structuredItems) - representedCount + len(rawFragments)
	items := make([]json.RawMessage, 0, total)
	structuredIndex := 0
	rawByIndex := make(map[int]llm.OpenAIResponsesRawFragment, len(rawFragments))
	for _, fragment := range rawFragments {
		if len(fragment.Raw) == 0 || fragment.OriginalIndex < 0 {
			return nil, false
		}
		rawByIndex[fragment.OriginalIndex] = fragment
	}

	for i := 0; i < total; i++ {
		if fragment, ok := rawByIndex[i]; ok {
			items = append(items, cloneRaw(fragment.Raw))
			structuredIndex += fragment.RepresentedToolCount
			continue
		}
		if structuredIndex >= len(structuredItems) {
			return nil, false
		}
		items = append(items, cloneRaw(structuredItems[structuredIndex]))
		structuredIndex++
	}

	if structuredIndex != len(structuredItems) {
		return nil, false
	}

	return items, true
}

func mergeRawOnlyTools(
	structuredRaw json.RawMessage,
	requestExt *llm.OpenAIResponsesRequestExtensions,
	currentTools []llm.Tool,
	replayRawInput bool,
) ([]json.RawMessage, bool, error) {
	if requestExt == nil || len(requestExt.RawTools) == 0 {
		return nil, false, nil
	}

	var structuredTools []json.RawMessage
	if len(structuredRaw) > 0 {
		if err := json.Unmarshal(structuredRaw, &structuredTools); err != nil {
			return nil, false, err
		}
	}

	rawGroups := buildRawToolGroups(requestExt.RawTools)
	plan, err := buildReplayPlan(currentTools, rawGroups, structuredTools, replayRawInput)
	if err != nil {
		return nil, false, err
	}
	if plan == nil {
		return nil, false, nil
	}

	tools := make([]json.RawMessage, 0, len(plan.steps))
	for _, step := range plan.steps {
		tools = append(tools, cloneRaw(step))
	}

	return tools, plan.replayed, nil
}

func rawToolChoiceMatchesCurrentTools(raw json.RawMessage, current *ToolChoice) bool {
	if current == nil {
		return false
	}

	var rawChoice ToolChoice
	if err := json.Unmarshal(raw, &rawChoice); err != nil {
		return false
	}

	return toolChoiceSignature(&rawChoice) == toolChoiceSignature(current)
}

func toolChoiceSignature(choice *ToolChoice) string {
	if choice == nil {
		return ""
	}

	if choice.Type != nil && *choice.Type == ToolChoiceTypeAllowedTools {
		data, err := json.Marshal(choice)
		if err != nil {
			return ""
		}
		return "allowed:" + string(data)
	}

	if choice.Mode != nil {
		return "mode:" + *choice.Mode
	}

	if choice.Type != nil && choice.Name != nil {
		return "named:" + *choice.Type + ":" + *choice.Name
	}

	return ""
}

func cloneRaw(src json.RawMessage) json.RawMessage {
	if len(src) == 0 {
		return nil
	}

	return append(json.RawMessage(nil), src...)
}
