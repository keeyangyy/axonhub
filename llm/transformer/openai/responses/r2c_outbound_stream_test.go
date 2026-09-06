package responses

import (
	"encoding/json"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/streams"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestOutboundTransformer_TransformStream_ToolSearchCallLifecycle(t *testing.T) {
	callID := "call_search_123"
	events := []*httpclient.StreamEvent{
		{Type: "response.created", Data: []byte(`{"type":"response.created","response":{"id":"resp_tool_search","object":"response","created_at":1700000000,"model":"gpt-5","status":"in_progress","output":[]}}`)},
		{Type: "response.output_item.added", Data: []byte(`{"type":"response.output_item.added","output_index":0,"item":{"id":"ts_123","type":"tool_search_call","status":"in_progress","call_id":"call_search_123","execution":"client","arguments":""}}`)},
		{Type: "response.function_call_arguments.delta", Data: []byte(`{"type":"response.function_call_arguments.delta","item_id":"ts_123","output_index":0,"delta":"{\"query\":"}`)},
		{Type: "response.function_call_arguments.delta", Data: []byte(`{"type":"response.function_call_arguments.delta","item_id":"ts_123","output_index":0,"delta":"\"agents\"}"}`)},
		{Type: "response.function_call_arguments.done", Data: []byte(`{"type":"response.function_call_arguments.done","item_id":"ts_123","output_index":0}`)},
		{Type: "response.output_item.done", Data: []byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"ts_123","type":"tool_search_call","status":"completed","call_id":"call_search_123","execution":"client"}}`)},
		{Type: "response.completed", Data: []byte(`{"type":"response.completed","response":{"id":"resp_tool_search","object":"response","created_at":1700000000,"model":"gpt-5","status":"completed","output":[]}}`)},
	}

	stream := newResponsesOutboundStream(streams.AppendStream(streams.SliceStream(events), lo.ToPtr(llm.DoneStreamEvent)))
	actual, err := streams.All(streams.NoNil(stream))
	require.NoError(t, err)
	require.Len(t, actual, 6)

	added := actual[1].Choices[0].Delta.ToolCalls[0]
	require.Equal(t, callID, added.ID)
	require.Equal(t, llm.ToolTypeResponsesToolSearch, added.Type)
	require.Equal(t, 0, added.Index)
	require.NotNil(t, added.ResponseToolSearchCall)
	require.Equal(t, callID, added.ResponseToolSearchCall.CallID)
	require.Equal(t, "client", added.ResponseToolSearchCall.Execution)

	var arguments strings.Builder
	for _, response := range actual[2:4] {
		require.Len(t, response.Choices, 1)
		require.NotNil(t, response.Choices[0].Delta)
		require.Len(t, response.Choices[0].Delta.ToolCalls, 1)
		delta := response.Choices[0].Delta.ToolCalls[0]
		require.Equal(t, llm.ToolTypeResponsesToolSearch, delta.Type)
		require.Equal(t, 0, delta.Index)
		require.NotNil(t, delta.ResponseToolSearchCall)
		require.Equal(t, callID, delta.ResponseToolSearchCall.CallID)
		require.Equal(t, "client", delta.ResponseToolSearchCall.Execution)
		arguments.WriteString(delta.ResponseToolSearchCall.Arguments)
	}
	require.JSONEq(t, `{"query":"agents"}`, arguments.String())
	require.NotNil(t, stream.state.toolCalls[callID])
	require.NotNil(t, stream.state.toolCalls[callID].ResponseToolSearchCall)
	require.JSONEq(t, `{"query":"agents"}`, stream.state.toolCalls[callID].ResponseToolSearchCall.Arguments)

	require.Equal(t, "tool_calls", lo.FromPtr(actual[4].Choices[0].FinishReason))
	require.Equal(t, llm.DoneResponse, actual[5])
}

func TestOutboundTransformer_TransformStream_PreservesCustomToolCallNamespace(t *testing.T) {
	events := []*httpclient.StreamEvent{
		{Type: "response.created", Data: []byte(`{"type":"response.created","response":{"id":"resp_custom_ns","object":"response","created_at":1700000000,"model":"gpt-5","status":"in_progress","output":[]}}`)},
		{Type: "response.output_item.added", Data: []byte(`{"type":"response.output_item.added","output_index":0,"item":{"id":"custom_ns","type":"custom_tool_call","status":"in_progress","call_id":"call_exec","namespace":"functions","name":"exec","input":""}}`)},
		{Type: "response.custom_tool_call_input.delta", Data: []byte(`{"type":"response.custom_tool_call_input.delta","item_id":"custom_ns","output_index":0,"delta":"ls"}`)},
		{Type: "response.custom_tool_call_input.done", Data: []byte(`{"type":"response.custom_tool_call_input.done","item_id":"custom_ns","output_index":0,"input":"ls"}`)},
		{Type: "response.output_item.done", Data: []byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"custom_ns","type":"custom_tool_call","status":"completed","call_id":"call_exec","namespace":"functions","name":"exec","input":"ls"}}`)},
		{Type: "response.completed", Data: []byte(`{"type":"response.completed","response":{"id":"resp_custom_ns","object":"response","created_at":1700000000,"model":"gpt-5","status":"completed","output":[]}}`)},
	}

	stream := newResponsesOutboundStream(streams.AppendStream(streams.SliceStream(events), lo.ToPtr(llm.DoneStreamEvent)))
	actual, err := streams.All(streams.NoNil(stream))
	require.NoError(t, err)
	require.NotEmpty(t, actual)

	for _, response := range actual {
		if response == llm.DoneResponse || len(response.Choices) == 0 || response.Choices[0].Delta == nil {
			continue
		}
		for _, call := range response.Choices[0].Delta.ToolCalls {
			if call.ResponseCustomToolCall != nil {
				require.Equal(t, "functions", call.ResponseCustomToolCall.Namespace)
			}
		}
	}
	require.NotNil(t, stream.state.toolCalls["call_exec"])
	require.Equal(t, "functions", stream.state.toolCalls["call_exec"].ResponseCustomToolCall.Namespace)
}

func TestOutboundTransformer_TransformStream_ToolSearchArgumentsProvidedOnlyInDone(t *testing.T) {
	trans, err := NewOutboundTransformer("https://api.openai.com", "test-api-key")
	require.NoError(t, err)

	callID := "call_search_done"
	events := []*httpclient.StreamEvent{
		{Data: []byte(`{"type":"response.created","response":{"id":"resp_ts_done","object":"response","created_at":1700000000,"model":"gpt-5","status":"in_progress","output":[]}}`)},
		{Data: []byte(`{"type":"response.output_item.added","output_index":0,"item":{"id":"ts_done","type":"tool_search_call","status":"in_progress","call_id":"call_search_done","execution":"client","arguments":""}}`)},
		{Data: []byte(`{"type":"response.function_call_arguments.done","item_id":"ts_done","output_index":0,"arguments":"{\"query\":\"agents\"}"}`)},
		{Data: []byte(`{"type":"response.completed","response":{"id":"resp_ts_done","object":"response","created_at":1700000000,"model":"gpt-5","status":"completed","output":[]}}`)},
	}

	stream, err := trans.TransformStream(t.Context(), nil, streams.SliceStream(events))
	require.NoError(t, err)

	responses, err := streams.All(stream)
	require.NoError(t, err)

	var arguments strings.Builder

	argumentChunks := 0

	for _, response := range responses {
		if response == llm.DoneResponse || len(response.Choices) == 0 || response.Choices[0].Delta == nil {
			continue
		}

		for _, toolCall := range response.Choices[0].Delta.ToolCalls {
			if toolCall.ResponseToolSearchCall == nil || toolCall.ResponseToolSearchCall.Arguments == "" {
				continue
			}

			argumentChunks++
			require.Equal(t, callID, toolCall.ResponseToolSearchCall.CallID)
			require.Equal(t, "client", toolCall.ResponseToolSearchCall.Execution)
			arguments.WriteString(toolCall.ResponseToolSearchCall.Arguments)
		}
	}

	require.Equal(t, 1, argumentChunks)
	require.JSONEq(t, `{"query":"agents"}`, arguments.String())
}

func TestOutboundTransformer_TransformStream_ToolSearchArgumentsProvidedOnlyInOutputItemDone(t *testing.T) {
	trans, err := NewOutboundTransformer("https://api.openai.com", "test-api-key")
	require.NoError(t, err)

	callID := "call_search_output_item_done"
	events := []*httpclient.StreamEvent{
		{Data: []byte(`{"type":"response.created","response":{"id":"resp_ts_item_done","object":"response","created_at":1700000000,"model":"gpt-5","status":"in_progress","output":[]}}`)},
		{Data: []byte(`{"type":"response.output_item.added","output_index":0,"item":{"id":"ts_item_done","type":"tool_search_call","status":"in_progress","call_id":"call_search_output_item_done","execution":"client","arguments":""}}`)},
		{Data: []byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"ts_item_done","type":"tool_search_call","status":"completed","call_id":"call_search_output_item_done","execution":"client","arguments":{"query":"agents"}}}`)},
		{Data: []byte(`{"type":"response.completed","response":{"id":"resp_ts_item_done","object":"response","created_at":1700000000,"model":"gpt-5","status":"completed","output":[]}}`)},
	}

	stream, err := trans.TransformStream(t.Context(), nil, streams.SliceStream(events))
	require.NoError(t, err)

	responses, err := streams.All(stream)
	require.NoError(t, err)

	var arguments strings.Builder
	argumentChunks := 0
	for _, response := range responses {
		if response == llm.DoneResponse || len(response.Choices) == 0 || response.Choices[0].Delta == nil {
			continue
		}
		for _, toolCall := range response.Choices[0].Delta.ToolCalls {
			if toolCall.ResponseToolSearchCall == nil || toolCall.ResponseToolSearchCall.Arguments == "" {
				continue
			}
			argumentChunks++
			require.Equal(t, callID, toolCall.ResponseToolSearchCall.CallID)
			require.Equal(t, "client", toolCall.ResponseToolSearchCall.Execution)
			arguments.WriteString(toolCall.ResponseToolSearchCall.Arguments)
		}
	}

	require.Equal(t, 1, argumentChunks)
	require.JSONEq(t, `{"query":"agents"}`, arguments.String())
}

func TestOutboundTransformer_TransformStream_ToolSearchExecutionProvidedOnlyInOutputItemDone(t *testing.T) {
	trans, err := NewOutboundTransformer("https://api.openai.com", "test-api-key")
	require.NoError(t, err)

	callID := "call_search_execution_done"
	events := []*httpclient.StreamEvent{
		{Data: []byte(`{"type":"response.created","response":{"id":"resp_ts_execution_done","object":"response","created_at":1700000000,"model":"gpt-5","status":"in_progress","output":[]}}`)},
		{Data: []byte(`{"type":"response.output_item.added","output_index":0,"item":{"id":"ts_execution_done","type":"tool_search_call","status":"in_progress","call_id":"call_search_execution_done","arguments":""}}`)},
		{Data: []byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"ts_execution_done","type":"tool_search_call","status":"completed","call_id":"call_search_execution_done","execution":"client"}}`)},
		{Data: []byte(`{"type":"response.completed","response":{"id":"resp_ts_execution_done","object":"response","created_at":1700000000,"model":"gpt-5","status":"completed","output":[]}}`)},
	}

	stream, err := trans.TransformStream(t.Context(), nil, streams.SliceStream(events))
	require.NoError(t, err)

	responses, err := streams.All(stream)
	require.NoError(t, err)

	var executionDelta *llm.ResponseToolSearchCall
	executionDeltas := 0
	for _, response := range responses {
		if response == llm.DoneResponse || len(response.Choices) == 0 || response.Choices[0].Delta == nil {
			continue
		}
		for _, toolCall := range response.Choices[0].Delta.ToolCalls {
			if toolCall.ResponseToolSearchCall != nil {
				if toolCall.ResponseToolSearchCall.Execution == "client" {
					require.Nil(t, executionDelta)
					executionDelta = toolCall.ResponseToolSearchCall
					executionDeltas++
				}
			}
		}
	}

	require.NotNil(t, executionDelta)
	require.Equal(t, 1, executionDeltas)
	require.Equal(t, callID, executionDelta.CallID)
	require.Equal(t, "client", executionDelta.Execution)
	require.Empty(t, executionDelta.Arguments)
}

func TestOutboundTransformer_TransformStream_CompletedWithUsageCarriesEchoFields(t *testing.T) {
	trans, err := NewOutboundTransformer("https://api.openai.com", "test-api-key")
	require.NoError(t, err)

	events := []*httpclient.StreamEvent{
		{Data: []byte(`{"type":"response.created","response":{"id":"resp_echo_usage","object":"response","created_at":1700000000,"model":"gpt-5","status":"in_progress","output":[],"conversation":{"id":"conv_echo"}}}`)},
		{Data: []byte(`{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"Hello"}`)},
		{Data: []byte(`{"type":"response.completed","response":{"id":"resp_echo_usage","object":"response","created_at":1700000000,"model":"gpt-5","status":"completed","output":[],"conversation":{"id":"conv_echo"},"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`)},
	}

	stream, err := trans.TransformStream(t.Context(), nil, streams.SliceStream(events))
	require.NoError(t, err)

	responses, err := streams.All(stream)
	require.NoError(t, err)

	var completedChunk *llm.Response
	var usageChunk *llm.Response

	for _, response := range responses {
		if response == llm.DoneResponse {
			continue
		}
		if len(response.Choices) > 0 && response.Choices[0].FinishReason != nil {
			completedChunk = response
		}
		if response.Usage != nil && len(response.Choices) == 0 {
			usageChunk = response
		}
	}

	require.NotNil(t, completedChunk)
	require.NotNil(t, usageChunk)
	require.NotEmpty(t, usageChunk.Usage)

	raw, ok := completedChunk.TransformerMetadata[responsesEchoFieldsTransformerMetadataKey]
	require.True(t, ok, "completed chunk must carry echo metadata on the usage path")

	echo, ok := raw.(json.RawMessage)
	require.True(t, ok)
	var decoded Response
	require.NoError(t, json.Unmarshal(echo, &decoded))
	require.NotNil(t, decoded.Conversation)
	require.Equal(t, "conv_echo", decoded.Conversation.ID)
}

func TestEchoFieldsSurviveTransformerMetadataJSONRoundTrip(t *testing.T) {
	echo := &Response{ID: "resp_echo", Conversation: &ResponseConversation{ID: "conv_echo"}}
	encoded, err := json.Marshal(echo)
	require.NoError(t, err)

	metadata := map[string]any{
		responsesEchoFieldsTransformerMetadataKey: json.RawMessage(encoded),
	}
	serialized, err := json.Marshal(metadata)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(serialized, &decoded))

	stream := &responsesInboundStream{}
	stream.captureEchoFields(decoded)
	require.NotNil(t, stream.echoResponse)
	require.Equal(t, "resp_echo", stream.echoResponse.ID)
	require.NotNil(t, stream.echoResponse.Conversation)
	require.Equal(t, "conv_echo", stream.echoResponse.Conversation.ID)
}

func TestEqualJSONValuesTreatsEquivalentNumbersAsEqual(t *testing.T) {
	require.True(t, equalJSONValues(`{"value":1}`, `{"value":1.0}`))
	require.True(t, equalJSONValues(`[1e2]`, `[100.0]`))
	require.False(t, equalJSONValues(`{"value":1}`, `{"value":2}`))
}
