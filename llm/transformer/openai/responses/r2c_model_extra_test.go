package responses

import (
	"encoding/json"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestItemMarshalJSON_ToolSearchCallRequiresObjectArguments(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
		expected  string
	}{
		{name: "null becomes empty object", arguments: "null", expected: `{}`},
		{name: "array becomes empty object", arguments: `["query"]`, expected: `{}`},
		{name: "object is preserved", arguments: `{"query":"agents"}`, expected: `{"query":"agents"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := Item{Type: "tool_search_call", CallID: "call_search", Arguments: tt.arguments}

			data, err := json.Marshal(item)
			require.NoError(t, err)

			var payload struct {
				Arguments json.RawMessage `json:"arguments"`
			}
			require.NoError(t, json.Unmarshal(data, &payload))
			require.JSONEq(t, tt.expected, string(payload.Arguments))
		})
	}
}

func TestResponseToolChoiceUnmarshalJSONErrorDoesNotMutate(t *testing.T) {
	invalidInputs := []string{`{`, `1`, `true`, `[]`, `["auto"]`}
	for _, input := range invalidInputs {
		t.Run(input, func(t *testing.T) {
			choice := ResponseToolChoice{
				StringValue: "required",
				ObjectValue: &ToolChoice{Type: lo.ToPtr("function"), Name: lo.ToPtr("lookup")},
			}
			before := choice

			before.ObjectValue = deepCopyToolChoice(choice.ObjectValue)

			require.Error(t, json.Unmarshal([]byte(input), &choice))
			require.Equal(t, before, choice)
		})
	}
}

func TestToolChoiceUnmarshalJSON_ClearsPreviousVariant(t *testing.T) {
	choice := ToolChoice{
		Type: lo.ToPtr("function"),
		Name: lo.ToPtr("lookup"),
	}

	require.NoError(t, json.Unmarshal([]byte(`"auto"`), &choice))
	require.Equal(t, "auto", lo.FromPtr(choice.Mode))
	require.Nil(t, choice.Type)
	require.Nil(t, choice.Name)
	require.Nil(t, choice.Tools)

	require.NoError(t, json.Unmarshal([]byte(`{"type":"function","name":"lookup"}`), &choice))
	require.Nil(t, choice.Mode)
	require.Equal(t, "function", lo.FromPtr(choice.Type))
	require.Equal(t, "lookup", lo.FromPtr(choice.Name))
	require.Nil(t, choice.Tools)

	require.NoError(t, json.Unmarshal([]byte(`{"type":"allowed_tools","mode":"required","tools":[]}`), &choice))
	require.Equal(t, "required", lo.FromPtr(choice.Mode))
	require.Equal(t, "allowed_tools", lo.FromPtr(choice.Type))
	require.Nil(t, choice.Name)
	require.NotNil(t, choice.Tools)
	require.Empty(t, choice.Tools)
}

func TestToolChoiceMarshalJSON_PreservesEmptyAllowedTools(t *testing.T) {
	cases := []struct {
		name     string
		mode     *string
		tools    []ToolOption
		expected string
	}{
		{
			name:     "nil mode and nil tools",
			expected: `{"type":"allowed_tools","tools":[]}`,
		},
		{
			name:     "auto with nil tools",
			mode:     lo.ToPtr("auto"),
			expected: `{"type":"allowed_tools","mode":"auto","tools":[]}`,
		},
		{
			name:     "auto with empty tools",
			mode:     lo.ToPtr("auto"),
			tools:    []ToolOption{},
			expected: `{"type":"allowed_tools","mode":"auto","tools":[]}`,
		},
		{
			name:     "required with nil tools",
			mode:     lo.ToPtr("required"),
			expected: `{"type":"allowed_tools","mode":"required","tools":[]}`,
		},
		{
			name:     "required with empty tools",
			mode:     lo.ToPtr("required"),
			tools:    []ToolOption{},
			expected: `{"type":"allowed_tools","mode":"required","tools":[]}`,
		},
		{
			name:     "type-only tool option omits name",
			mode:     lo.ToPtr("required"),
			tools:    []ToolOption{{Type: "image_generation"}},
			expected: `{"type":"allowed_tools","mode":"required","tools":[{"type":"image_generation"}]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			choice := ToolChoice{Type: lo.ToPtr("allowed_tools"), Mode: tc.mode, Tools: tc.tools}
			data, err := json.Marshal(&choice)
			require.NoError(t, err)
			require.JSONEq(t, tc.expected, string(data))
		})
	}
}

func TestToolChoiceJSONPrimitiveMatrix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, choice ToolChoice)
	}{
		{
			name:  "auto mode",
			input: `"auto"`,
			validate: func(t *testing.T, choice ToolChoice) {
				require.Equal(t, "auto", lo.FromPtr(choice.Mode))
				require.Nil(t, choice.Type)
				require.Nil(t, choice.Name)
			},
		},
		{
			name:  "named function",
			input: `{"type":"function","name":"lookup"}`,
			validate: func(t *testing.T, choice ToolChoice) {
				require.Equal(t, "function", lo.FromPtr(choice.Type))
				require.Equal(t, "lookup", lo.FromPtr(choice.Name))
			},
		},
		{
			name:  "named custom",
			input: `{"type":"custom","name":"apply_patch"}`,
			validate: func(t *testing.T, choice ToolChoice) {
				require.Equal(t, "custom", lo.FromPtr(choice.Type))
				require.Equal(t, "apply_patch", lo.FromPtr(choice.Name))
			},
		},
		{
			name:  "named namespace",
			input: `{"type":"namespace","name":"collaboration"}`,
			validate: func(t *testing.T, choice ToolChoice) {
				require.Equal(t, "namespace", lo.FromPtr(choice.Type))
				require.Equal(t, "collaboration", lo.FromPtr(choice.Name))
			},
		},
		{
			name:  "named tool search",
			input: `{"type":"tool_search","name":"discover"}`,
			validate: func(t *testing.T, choice ToolChoice) {
				require.Equal(t, "tool_search", lo.FromPtr(choice.Type))
				require.Equal(t, "discover", lo.FromPtr(choice.Name))
			},
		},
		{
			name:  "named future client primitive",
			input: `{"type":"future_client_tool","name":"later"}`,
			validate: func(t *testing.T, choice ToolChoice) {
				require.Equal(t, "future_client_tool", lo.FromPtr(choice.Type))
				require.Equal(t, "later", lo.FromPtr(choice.Name))
			},
		},
		{
			name:  "type only hosted selector",
			input: `{"type":"web_search"}`,
			validate: func(t *testing.T, choice ToolChoice) {
				require.Equal(t, "web_search", lo.FromPtr(choice.Type))
				require.Nil(t, choice.Name)
			},
		},
		{
			name: "allowed primitive matrix keeps type name and order",
			input: `{"type":"allowed_tools","mode":"required","tools":[
				{"type":"function","name":"same"},
				{"type":"custom","name":"same"},
				{"type":"namespace","name":"workspace"},
				{"type":"tool_search","name":"discover"},
				{"type":"future_client_tool","name":"later"},
				{"type":"future_server_tool","name":"hosted"}
			]}`,
			validate: func(t *testing.T, choice ToolChoice) {
				require.Equal(t, "allowed_tools", lo.FromPtr(choice.Type))
				require.Equal(t, "required", lo.FromPtr(choice.Mode))
				require.Equal(t, []ToolOption{
					{Type: "function", Name: "same"},
					{Type: "custom", Name: "same"},
					{Type: "namespace", Name: "workspace"},
					{Type: "tool_search", Name: "discover"},
					{Type: "future_client_tool", Name: "later"},
					{Type: "future_server_tool", Name: "hosted"},
				}, choice.Tools)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var decoded ToolChoice
			require.NoError(t, json.Unmarshal([]byte(tt.input), &decoded))
			tt.validate(t, decoded)

			encoded, err := json.Marshal(&decoded)
			require.NoError(t, err)
			require.JSONEq(t, tt.input, string(encoded))

			var roundTripped ToolChoice
			require.NoError(t, json.Unmarshal(encoded, &roundTripped))
			require.Equal(t, decoded, roundTripped)
		})
	}
}

func TestToolChoiceUnmarshalJSONErrorDoesNotMutate(t *testing.T) {
	invalidInputs := []string{`{`, `1`, `true`, `[]`, `["auto"]`}
	for _, input := range invalidInputs {
		t.Run(input, func(t *testing.T) {
			choice := ToolChoice{
				Mode:  lo.ToPtr("required"),
				Type:  lo.ToPtr("allowed_tools"),
				Name:  lo.ToPtr("stale"),
				Tools: []ToolOption{{Type: "function", Name: "lookup"}},
			}

			before := *deepCopyToolChoice(&choice)

			require.Error(t, json.Unmarshal([]byte(input), &choice))
			require.Equal(t, lo.FromPtr(before.Mode), lo.FromPtr(choice.Mode))
			require.Equal(t, lo.FromPtr(before.Type), lo.FromPtr(choice.Type))
			require.Equal(t, lo.FromPtr(before.Name), lo.FromPtr(choice.Name))
			require.Len(t, choice.Tools, len(before.Tools))
			for i := range before.Tools {
				require.Equal(t, before.Tools[i], choice.Tools[i])
			}
		})
	}
}

// deepCopyToolChoice returns a snapshot of tc that shares no pointer fields
// or slice backing array with the original, for mutation assertions.
func deepCopyToolChoice(tc *ToolChoice) *ToolChoice {
	if tc == nil {
		return nil
	}
	cp := *tc
	if tc.Mode != nil {
		cp.Mode = lo.ToPtr(*tc.Mode)
	}
	if tc.Type != nil {
		cp.Type = lo.ToPtr(*tc.Type)
	}
	if tc.Name != nil {
		cp.Name = lo.ToPtr(*tc.Name)
	}
	cp.Tools = append([]ToolOption(nil), tc.Tools...)
	return &cp
}

// FuzzToolChoiceJSONRoundTrip fuzzes ToolChoice (not ResponseToolChoice).
// It asserts that encoding is idempotent: re-encoding a decoded value is
// stable. It does not assert fidelity to the original raw input.
func FuzzToolChoiceJSONRoundTrip(f *testing.F) {
	seeds := []string{
		`"auto"`,
		`{"type":"function","name":"lookup"}`,
		`{"type":"custom","name":"apply_patch"}`,
		`{"type":"future_client_tool","name":"later"}`,
		`{"type":"web_search"}`,
		`{"type":"mcp","server_label":"docs","name":"search"}`,
		`{"type":"allowed_tools","mode":"auto","tools":[]}`,
		`{"type":"allowed_tools","mode":"required","tools":[{"type":"function","name":"lookup"},{"type":"custom","name":"apply_patch"}]}`,
		`null`,
		`{}`,
		`{`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		var decoded ToolChoice
		if err := json.Unmarshal([]byte(input), &decoded); err != nil {
			return
		}

		encoded, err := json.Marshal(&decoded)
		require.NoError(t, err)
		var roundTripped ToolChoice
		require.NoError(t, json.Unmarshal(encoded, &roundTripped))
		reencoded, err := json.Marshal(&roundTripped)
		require.NoError(t, err)
		require.JSONEq(t, string(encoded), string(reencoded))
	})
}
