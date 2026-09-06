package openai

import (
	"encoding/json"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/auth"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOutboundTransformer_TransformRequest_ReasoningOnlyAssistantKeepsContentField(t *testing.T) {
	transformer, err := NewOutboundTransformerWithConfig(&Config{
		PlatformType:   PlatformOpenAI,
		BaseURL:        "https://api.openai.com/v1",
		APIKeyProvider: auth.NewStaticKeyProvider("test-key"),
	})
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	req, err := transformer.TransformRequest(t.Context(), &llm.Request{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("hello")}},
			{Role: "assistant", ReasoningContent: lo.ToPtr("thinking")},
			{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("continue")}},
		},
	})
	assert.NoError(t, err)

	var raw struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(req.Body, &raw))
	require.Len(t, raw.Messages, 3)

	content, hasContent := raw.Messages[1]["content"]
	require.True(t, hasContent, "reasoning-only assistant message must keep the content key on the wire")
	assert.Equal(t, `""`, string(content))
}
