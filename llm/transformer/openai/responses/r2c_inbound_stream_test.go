package responses

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/streams"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
)

func assertParallelFunctionCallLifecycle(t *testing.T, events []StreamEvent) {
	t.Helper()

	type lifecycle struct {
		addedCount     int
		deltaArguments string
		lastDeltaIndex int
		argumentsDone  int
		outputDone     int
		arguments      string
		argumentsIndex int
		outputIndex    int
	}
	calls := map[int]*lifecycle{2: {}, 3: {}}

	for i := range events {
		event := events[i]
		call, tracked := calls[event.OutputIndex]
		if !tracked {
			continue
		}
		switch event.Type {
		case StreamEventTypeOutputItemAdded:
			if event.Item != nil && event.Item.Type == "function_call" {
				call.addedCount++
			}
		case StreamEventTypeFunctionCallArgumentsDelta:
			call.deltaArguments += event.Delta
			call.lastDeltaIndex = i
		case StreamEventTypeFunctionCallArgumentsDone:
			call.argumentsDone++
			call.arguments = event.Arguments
			call.argumentsIndex = i
		case StreamEventTypeOutputItemDone:
			if event.Item != nil && event.Item.Type == "function_call" {
				call.outputDone++
				call.outputIndex = i
				require.Equal(t, event.Item.Arguments, call.arguments)
			}
		}
	}

	wantArguments := map[int]string{2: `{"expression":"25 * 4"}`, 3: `{"location":"Tokyo"}`}
	for outputIndex, call := range calls {
		require.Equal(t, 1, call.addedCount, "output index %d added count", outputIndex)
		require.Equal(t, wantArguments[outputIndex], call.deltaArguments, "output index %d delta arguments", outputIndex)
		require.Equal(t, 1, call.argumentsDone, "output index %d arguments.done count", outputIndex)
		require.Equal(t, 1, call.outputDone, "output index %d output_item.done count", outputIndex)
		require.Equal(t, wantArguments[outputIndex], call.arguments, "output index %d completed arguments", outputIndex)
		require.Greater(t, call.argumentsIndex, call.lastDeltaIndex, "output index %d arguments.done ordering", outputIndex)
		require.Greater(t, call.outputIndex, call.argumentsIndex, "output index %d output_item.done ordering", outputIndex)
	}
}

func TestInboundTransformer_TransformStream_UsesStableItemIDsForParallelToolCallDeltas(t *testing.T) {
	tests := []struct {
		name    string
		deltaID func(int) string
	}{
		{name: "missing IDs", deltaID: func(int) string { return "" }},
		{name: "delayed IDs", deltaID: func(index int) string { return fmt.Sprintf("late_call_%d", index) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := streams.SliceStream([]*llm.Response{
				{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{ToolCalls: []llm.ToolCall{
					{Index: 0, Function: llm.FunctionCall{Name: "lookup"}},
					{Index: 1, ResponseCustomToolCall: &llm.ResponseCustomToolCall{CallID: "custom_call", Name: "apply_patch"}},
					{Index: 2, ResponseToolSearchCall: &llm.ResponseToolSearchCall{CallID: "search_call", Execution: "client"}},
				}}}}},
				{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{ToolCalls: []llm.ToolCall{
					{ID: tt.deltaID(0), Index: 0, Function: llm.FunctionCall{Arguments: `{"query":"docs"}`}},
					{ID: tt.deltaID(1), Index: 1, ResponseCustomToolCall: &llm.ResponseCustomToolCall{Input: "patch"}},
					{ID: tt.deltaID(2), Index: 2, ResponseToolSearchCall: &llm.ResponseToolSearchCall{Arguments: `{"query":"agents"}`}},
				}}}}},
				{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{}, FinishReason: lo.ToPtr("tool_calls")}}},
				llm.DoneResponse,
			})

			stream, err := NewInboundTransformer().TransformStream(t.Context(), source)
			require.NoError(t, err)
			addedItemIDs := make(map[int]string)
			var deltaEvents []StreamEvent
			for stream.Next() {
				var event StreamEvent
				require.NoError(t, json.Unmarshal(stream.Current().Data, &event))
				if event.Type == StreamEventTypeOutputItemAdded && event.Item != nil {
					addedItemIDs[event.OutputIndex] = event.Item.ID
				}
				if event.Type == StreamEventTypeFunctionCallArgumentsDelta || event.Type == StreamEventTypeCustomToolCallInputDelta {
					deltaEvents = append(deltaEvents, event)
				}
			}
			require.NoError(t, stream.Err())
			require.Len(t, addedItemIDs, 3)
			require.Len(t, deltaEvents, 3)
			deltaCounts := make(map[int]int, len(deltaEvents))
			for _, event := range deltaEvents {
				deltaCounts[event.OutputIndex]++
			}
			require.Equal(t, map[int]int{0: 1, 1: 1, 2: 1}, deltaCounts)
			for _, event := range deltaEvents {
				require.NotNil(t, event.ItemID)
				require.Equal(t, addedItemIDs[event.OutputIndex], *event.ItemID, "output index %d item ID", event.OutputIndex)
			}
		})
	}
}

func TestInboundTransformer_TransformStream_ClosesParallelToolCallsInIndexOrder(t *testing.T) {
	source := streams.SliceStream([]*llm.Response{
		{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{ToolCalls: []llm.ToolCall{
			{ID: "call_2", Index: 2, Function: llm.FunctionCall{Name: "third"}},
			{ID: "call_0", Index: 0, Function: llm.FunctionCall{Name: "first"}},
			{ID: "call_1", Index: 1, Function: llm.FunctionCall{Name: "second"}},
		}}}}},
		{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{}, FinishReason: lo.ToPtr("tool_calls")}}},
		llm.DoneResponse,
	})

	stream, err := NewInboundTransformer().TransformStream(t.Context(), source)
	require.NoError(t, err)
	var doneCallIDs []string
	for stream.Next() {
		var event StreamEvent
		require.NoError(t, json.Unmarshal(stream.Current().Data, &event))
		if event.Type == StreamEventTypeOutputItemDone && event.Item != nil && event.Item.Type == "function_call" {
			doneCallIDs = append(doneCallIDs, event.Item.CallID)
		}
	}
	require.NoError(t, stream.Err())
	require.Equal(t, []string{"call_0", "call_1", "call_2"}, doneCallIDs)
}

func TestInboundTransformer_TransformStream_EmitsAdaptedSpecialToolCalls(t *testing.T) {
	wrappedMetadata := map[string]any{ChatWrappedCustomMetadataKey: true}
	source := streams.SliceStream([]*llm.Response{
		{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{Role: "assistant"}}}},
		{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{ToolCalls: []llm.ToolCall{
			{ID: "call_custom", Index: 0, ResponseCustomToolCall: &llm.ResponseCustomToolCall{CallID: "call_custom", Name: "apply_patch", Input: `{"input":"*** Begin`}, TransformerMetadata: wrappedMetadata},
			{ID: "call_search", Index: 1, ResponseToolSearchCall: &llm.ResponseToolSearchCall{CallID: "call_search", Execution: "client", Arguments: `{"query":"agents"}`}},
		}}}}},
		{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{ToolCalls: []llm.ToolCall{
			{Index: 0, ResponseCustomToolCall: &llm.ResponseCustomToolCall{Input: ` Patch"}`}},
		}}}}},
		{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{}, FinishReason: lo.ToPtr("tool_calls")}}},
		llm.DoneResponse,
	})

	stream, err := NewInboundTransformer().TransformStream(t.Context(), source)
	require.NoError(t, err)
	var events []StreamEvent
	for stream.Next() {
		var event StreamEvent
		require.NoError(t, json.Unmarshal(stream.Current().Data, &event))
		events = append(events, event)
	}
	require.NoError(t, stream.Err())

	var customDone, searchDone *Item
	for i := range events {
		event := events[i]
		if event.Type == StreamEventTypeCustomToolCallInputDelta {
			require.NotContains(t, event.Delta, `{"input"`)
		}
		if event.Type != StreamEventTypeOutputItemDone || event.Item == nil {
			continue
		}
		switch event.Item.Type {
		case "custom_tool_call":
			customDone = event.Item
		case "tool_search_call":
			searchDone = event.Item
		}
	}
	require.NotNil(t, customDone)
	require.Equal(t, "*** Begin Patch", lo.FromPtr(customDone.Input))
	require.NotNil(t, searchDone)
	require.Equal(t, "client", searchDone.Execution)
	require.JSONEq(t, `{"query":"agents"}`, searchDone.Arguments)
}

func TestInboundTransformer_TransformStream_RejectsToolCallTypeChanges(t *testing.T) {
	plain := llm.ToolCall{ID: "call_1", Index: 0, Type: "function", Function: llm.FunctionCall{Name: "lookup"}}
	custom := llm.ToolCall{ID: "call_1", Index: 0, ResponseCustomToolCall: &llm.ResponseCustomToolCall{CallID: "call_1", Name: "apply_patch"}}
	toolSearch := llm.ToolCall{ID: "call_1", Index: 0, ResponseToolSearchCall: &llm.ResponseToolSearchCall{CallID: "call_1", Execution: "client"}}
	tests := []struct {
		name   string
		first  llm.ToolCall
		second llm.ToolCall
		want   string
	}{
		{name: "function to custom", first: plain, second: custom, want: "from function to custom"},
		{name: "function to tool search", first: plain, second: toolSearch, want: "from function to tool_search"},
		{name: "custom to function", first: custom, second: plain, want: "from custom to function"},
		{name: "tool search to function", first: toolSearch, second: plain, want: "from tool_search to function"},
		{name: "custom to tool search", first: custom, second: toolSearch, want: "from custom to tool_search"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := streams.SliceStream([]*llm.Response{
				{ID: "resp_changed_type", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{ToolCalls: []llm.ToolCall{tt.first}}}}},
				{ID: "resp_changed_type", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{ToolCalls: []llm.ToolCall{tt.second}}}}},
				llm.DoneResponse,
			})
			stream, err := NewInboundTransformer().TransformStream(t.Context(), source)
			require.NoError(t, err)
			for stream.Next() {
				_ = stream.Current()
			}
			require.ErrorContains(t, stream.Err(), "tool call index 0 changed type "+tt.want)
		})
	}
}

func TestInboundTransformer_TransformStream_DegradesMalformedWrappedCustomInput(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	wrappedMetadata := map[string]any{ChatWrappedCustomMetadataKey: true}
	source := streams.SliceStream([]*llm.Response{
		{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{ToolCalls: []llm.ToolCall{{
			ID: "call_custom", Index: 0,
			ResponseCustomToolCall: &llm.ResponseCustomToolCall{CallID: "call_custom", Name: "apply_patch", Input: `{"input":"patch"`},
			TransformerMetadata:    wrappedMetadata,
		}}}}}},
		{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{}, FinishReason: lo.ToPtr("tool_calls")}}},
		llm.DoneResponse,
	})

	stream, err := NewInboundTransformer().TransformStream(t.Context(), source)
	require.NoError(t, err)
	var encodedEvents []string
	for stream.Next() {
		encodedEvents = append(encodedEvents, string(stream.Current().Data))
	}
	require.NoError(t, stream.Err())
	require.Contains(t, logs.String(), "failed to unwrap Chat custom tool input")
	require.Contains(t, logs.String(), "call_custom")
	for _, event := range encodedEvents {
		require.NotContains(t, event, `{"input":"patch"`)
	}
}

func TestInboundTransformer_TransformStream_WarnsOnMissingWrappedCustomInput(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	wrappedMetadata := map[string]any{ChatWrappedCustomMetadataKey: true}
	source := streams.SliceStream([]*llm.Response{
		{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{ToolCalls: []llm.ToolCall{{
			ID: "call_custom", Index: 0,
			ResponseCustomToolCall: &llm.ResponseCustomToolCall{CallID: "call_custom", Name: "apply_patch", Input: `{}`},
			TransformerMetadata:    wrappedMetadata,
		}}}}}},
		{ID: "resp_tools", Model: "glm-5.2", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{}, FinishReason: lo.ToPtr("tool_calls")}}},
		llm.DoneResponse,
	})

	stream, err := NewInboundTransformer().TransformStream(t.Context(), source)
	require.NoError(t, err)
	for stream.Next() {
		_ = stream.Current()
	}
	require.NoError(t, stream.Err())
	require.Contains(t, logs.String(), "failed to unwrap Chat custom tool input")
	require.Contains(t, logs.String(), "missing input field")
}

func isResponsesTerminalEventType(eventType StreamEventType) bool {
	switch eventType {
	case StreamEventTypeResponseCompleted,
		StreamEventTypeResponseIncomplete,
		StreamEventTypeResponseFailed,
		StreamEventTypeResponseCancelled:
		return true
	default:
		return false
	}
}

func TestInboundTransformer_TransformStream_CompletesWhenFinishReasonMissing(t *testing.T) {
	source := streams.SliceStream([]*llm.Response{
		{ID: "resp_1", Model: "kimi-k3", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{Content: llm.MessageContent{Content: lo.ToPtr("dispatching")}}}}},
		{ID: "resp_1", Model: "kimi-k3", Choices: []llm.Choice{{Index: 0, Delta: &llm.Message{ToolCalls: []llm.ToolCall{{
			ID: "call_task", Index: 0, Type: llm.ToolTypeFunction,
			Function: llm.FunctionCall{Name: "Task", Arguments: `{"prompt":"fix the bug"}`},
		}}}}}},
		llm.DoneResponse,
	})

	stream, err := NewInboundTransformer().TransformStream(t.Context(), source)
	require.NoError(t, err)

	var terminal *StreamEvent
	closedTypes := make([]string, 0)
	for stream.Next() {
		var event StreamEvent
		require.NoError(t, json.Unmarshal(stream.Current().Data, &event))
		switch event.Type {
		case StreamEventTypeResponseCompleted:
			terminal = &event
		case StreamEventTypeOutputItemDone:
			if event.Item != nil {
				closedTypes = append(closedTypes, event.Item.Type)
			}
		}
	}
	require.NoError(t, stream.Err())

	require.NotNil(t, terminal, "stream must emit a terminal response when the upstream omits finish_reason")
	require.NotNil(t, terminal.Response)
	require.Equal(t, "completed", lo.FromPtr(terminal.Response.Status))
	require.Contains(t, closedTypes, "message")
	require.Contains(t, closedTypes, "function_call")

	var taskItem *Item
	for i := range terminal.Response.Output {
		if terminal.Response.Output[i].Type == "function_call" {
			taskItem = &terminal.Response.Output[i]
			break
		}
	}
	require.NotNil(t, taskItem, "terminal response must keep the dispatched tool call")
	require.Equal(t, "Task", taskItem.Name)
	require.Equal(t, `{"prompt":"fix the bug"}`, taskItem.Arguments)
}

func TestInboundTransformer_TransformStream_EmitsErrorWhenUpstreamEndsWithoutResponseCreated(t *testing.T) {
	source := streams.SliceStream([]*llm.Response{})

	stream, err := NewInboundTransformer().TransformStream(t.Context(), source)
	require.NoError(t, err)

	var events []StreamEvent
	for stream.Next() {
		var event StreamEvent
		require.NoError(t, json.Unmarshal(stream.Current().Data, &event))
		events = append(events, event)
	}
	require.NoError(t, stream.Err())

	require.Len(t, events, 1)
	require.Equal(t, StreamEventTypeError, events[0].Type)
	require.NotEmpty(t, events[0].Message)
}
