package responses

import (
	"context"
	"encoding/json"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestOutboundTransformer_TransformRequest_DoesNotMutateRetryMetadata(t *testing.T) {
	transformer, err := NewOutboundTransformer("https://api.openai.com", "test-api-key")
	require.NoError(t, err)

	request := &llm.Request{
		Model:               "gpt-5.6",
		TransformerMetadata: map[string]any{"caller": "kept"},
		Messages: []llm.Message{{
			Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("generate an image")},
		}},
		Tools: []llm.Tool{{
			Type: llm.ToolTypeImageGeneration,
			ImageGeneration: &llm.ImageGeneration{
				OutputFormat: "png",
			},
		}},
	}
	httpRequest, err := transformer.TransformRequest(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"caller": "kept"}, request.TransformerMetadata)
	require.NotContains(t, request.TransformerMetadata, "image_output_format")
	require.Equal(t, "png", httpRequest.TransformerMetadata["image_output_format"])
}

func TestOutboundTransformer_TransformRequest_ReplaysFutureFunctionLikeTool(t *testing.T) {
	inbound := NewInboundTransformer()
	llmReq, err := inbound.TransformRequest(context.Background(), &httpclient.Request{Body: []byte(`{
		"model":"gpt-4o","input":"lookup","tools":[{
			"type":"future_client_tool","name":"lookup","execution":"client",
			"future_option":{"mode":"fast"},
			"parameters":{"type":"object","properties":{"query":{"type":"string"}}}
		}]
	}`)})
	require.NoError(t, err)
	require.Len(t, llmReq.Tools, 1)
	require.Equal(t, llm.ToolTypeFunction, llmReq.Tools[0].Type)

	outbound, err := NewOutboundTransformer("https://api.openai.com", "test-api-key")
	require.NoError(t, err)
	httpReq, err := outbound.TransformRequest(context.Background(), llmReq)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(httpReq.Body, &payload))
	tools := payload["tools"].([]any)
	require.Len(t, tools, 1)
	tool := tools[0].(map[string]any)
	require.Equal(t, "future_client_tool", tool["type"])
	require.Equal(t, "lookup", tool["name"])
	require.Equal(t, "fast", tool["future_option"].(map[string]any)["mode"])
}

func TestOutboundTransformer_TransformRequest_CombinesNonAdjacentNamespaceMembers(t *testing.T) {
	llmReq := &llm.Request{Tools: []llm.Tool{
		{Type: llm.ToolTypeFunction, Function: llm.Function{Name: "functions__one", Namespace: "functions", Parameters: []byte(`{"type":"object"}`)}},
		{Type: llm.ToolTypeFunction, Function: llm.Function{Name: "mid", Parameters: []byte(`{"type":"object"}`)}},
		{Type: llm.ToolTypeFunction, Function: llm.Function{Name: "functions__two", Namespace: "functions", Parameters: []byte(`{"type":"object"}`)}},
	}}

	outbound, err := NewOutboundTransformer("https://api.openai.com", "test-api-key")
	require.NoError(t, err)
	httpReq, err := outbound.TransformRequest(context.Background(), llmReq)
	require.NoError(t, err)

	var payload struct {
		Tools []Tool `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(httpReq.Body, &payload))
	require.Len(t, payload.Tools, 2)
	require.Equal(t, "namespace", payload.Tools[0].Type)
	require.Equal(t, "functions", payload.Tools[0].Name)
	require.Len(t, payload.Tools[0].Tools, 2)
	require.Equal(t, "one", payload.Tools[0].Tools[0].Name)
	require.Equal(t, "two", payload.Tools[0].Tools[1].Name)
	require.Equal(t, "function", payload.Tools[1].Type)
	require.Equal(t, "mid", payload.Tools[1].Name)
}

func TestOutboundTransformer_TransformRequest_RejectsConflictingNamespaceDescriptions(t *testing.T) {
	llmReq := &llm.Request{Tools: []llm.Tool{
		{
			Type: llm.ToolTypeFunction, Function: llm.Function{Name: "functions__one", Namespace: "functions"},
			ResponsesNamespaceDescription: "first",
		},
		{
			Type: llm.ToolTypeFunction, Function: llm.Function{Name: "functions__two", Namespace: "functions"},
			ResponsesNamespaceDescription: "second",
		},
	}}

	outbound, err := NewOutboundTransformer("https://api.openai.com", "test-api-key")
	require.NoError(t, err)
	httpReq, err := outbound.TransformRequest(context.Background(), llmReq)
	require.Nil(t, httpReq)
	require.ErrorContains(t, err, `namespace_description_conflict: namespace "functions" has multiple descriptions`)
}

func TestOutboundTransformer_TransformRequest_RejectsNonCanonicalNamespaceFunctionName(t *testing.T) {
	llmReq := &llm.Request{Tools: []llm.Tool{{
		Type:     llm.ToolTypeFunction,
		Function: llm.Function{Name: "exec", Namespace: "functions", Parameters: []byte(`{"type":"object"}`)},
	}}}

	outbound, err := NewOutboundTransformer("https://api.openai.com", "test-api-key")
	require.NoError(t, err)
	httpReq, err := outbound.TransformRequest(context.Background(), llmReq)
	require.Nil(t, httpReq)
	require.ErrorContains(t, err, `invalid_namespace_tool: function "exec" in namespace "functions" must use flattened name "functions__<name>"`)
}

func TestOutboundTransformer_TransformResponse_PreservesCustomToolCallNamespace(t *testing.T) {
	transformer, err := NewOutboundTransformer("https://api.openai.com", "test-api-key")
	require.NoError(t, err)

	result, err := transformer.TransformResponse(context.Background(), &httpclient.Response{
		StatusCode: http.StatusOK,
		Body: []byte(`{
			"id": "resp_namespace_custom",
			"object": "response",
			"created_at": 1765086000,
			"status": "completed",
			"model": "gpt-5.5",
			"output": [{
				"type": "custom_tool_call",
				"call_id": "call_exec",
				"namespace": "functions",
				"name": "exec",
				"input": "ls"
			}]
		}`),
	})
	require.NoError(t, err)
	require.Len(t, result.Choices, 1)
	require.Len(t, result.Choices[0].Message.ToolCalls, 1)
	customCall := result.Choices[0].Message.ToolCalls[0].ResponseCustomToolCall
	require.NotNil(t, customCall)
	require.Equal(t, "functions", customCall.Namespace)
}
