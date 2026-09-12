package openai

import (
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/llm"
)

// Tool results are converted as-is, matching upstream. Text-only results still
// serialize as a string, but a result carrying an image keeps its parts so the
// model actually receives the image. Codex's view_image returns an image with
// no text at all, which must never be flattened into an empty result string.
func TestMessageFromLLMToolContentPassThrough(t *testing.T) {
	imagePart := llm.MessageContentPart{
		Type:     "image_url",
		ImageURL: &llm.ImageURL{URL: "data:image/png;base64,iVBORw0KGgo=", Detail: lo.ToPtr("auto")},
	}

	tests := []struct {
		name            string
		content         llm.MessageContent
		wantParts       int
		wantContains    []string
		wantScalarValue string
	}{
		{
			name:            "image only (codex view_image)",
			content:         llm.MessageContent{MultipleContent: []llm.MessageContentPart{imagePart}},
			wantParts:       1,
			wantContains:    []string{`"image_url"`, `"data:image/png;base64,iVBORw0KGgo="`, `"detail":"auto"`},
			wantScalarValue: "",
		},
		{
			name: "text and image",
			content: llm.MessageContent{MultipleContent: []llm.MessageContentPart{
				{Type: "text", Text: lo.ToPtr("screenshot attached")},
				imagePart,
			}},
			wantParts:    2,
			wantContains: []string{`"screenshot attached"`, `"image_url"`},
		},
		{
			name:            "single text result stays a string",
			content:         llm.MessageContent{Content: lo.ToPtr("command finished")},
			wantParts:       0,
			wantContains:    nil,
			wantScalarValue: `"command finished"`,
		},
		{
			name: "multiple text parts stay an array",
			content: llm.MessageContent{MultipleContent: []llm.MessageContentPart{
				{Type: "text", Text: lo.ToPtr("first")},
				{Type: "text", Text: lo.ToPtr("second")},
			}},
			wantParts:    2,
			wantContains: []string{`"first"`, `"second"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := MessageFromLLM(llm.Message{
				Role:       "tool",
				ToolCallID: lo.ToPtr("call_1"),
				Content:    tt.content,
			})

			raw, err := json.Marshal(msg.Content)
			require.NoError(t, err)

			if tt.wantScalarValue != "" {
				require.Equal(t, tt.wantScalarValue, string(raw))

				return
			}

			require.Len(t, msg.Content.MultipleContent, tt.wantParts)

			for _, want := range tt.wantContains {
				require.Contains(t, string(raw), want)
			}

			// The wire shape must be an array, otherwise the non-text parts
			// were flattened away before reaching the provider.
			var wire any
			require.NoError(t, json.Unmarshal(raw, &wire))

			_, isArray := wire.([]any)
			require.Truef(t, isArray, "expected array wire shape, got %T: %s", wire, raw)
		})
	}
}
