package llm_test

import (
	"encoding/json"
	"github.com/looplj/axonhub/llm"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestToolChoiceAllowedToolsJSONRoundTrip(t *testing.T) {
	auto := "auto"
	choice := llm.ToolChoice{
		ToolChoice: &auto, AllowedToolsSet: true,
		AllowedTools: []llm.ToolOption{{Type: "function", Name: "lookup"}},
	}
	data, err := json.Marshal(choice)
	require.NoError(t, err)
	require.JSONEq(t, `{"mode":"auto","tools":[{"type":"function","name":"lookup"}]}`, string(data))

	var decoded llm.ToolChoice
	require.NoError(t, json.Unmarshal([]byte(`{"mode":"auto","tools":[]}`), &decoded))
	require.True(t, decoded.AllowedToolsSet)
	require.Equal(t, "auto", *decoded.ToolChoice)
	require.Empty(t, decoded.AllowedTools)
}

func TestToolChoiceNilAllowedToolsJSONRoundTrip(t *testing.T) {
	choice := llm.ToolChoice{AllowedToolsSet: true}

	data, err := json.Marshal(choice)
	require.NoError(t, err)
	require.JSONEq(t, `{"tools":[]}`, string(data))

	var decoded llm.ToolChoice
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.True(t, decoded.AllowedToolsSet)
	require.NotNil(t, decoded.AllowedTools)
	require.Empty(t, decoded.AllowedTools)
}

func TestNamespaceFunctionNameHelpers(t *testing.T) {
	require.Equal(t, "lookup", llm.JoinNamespaceFunctionName("", "lookup"))
	require.Equal(t, "functions__lookup", llm.JoinNamespaceFunctionName("functions", "lookup"))
	require.Equal(t, "a__b__c", llm.JoinNamespaceFunctionName("a__b", "c"))

	member, err := llm.ValidateNamespaceFunctionName("functions", "functions__lookup")
	require.NoError(t, err)
	require.Equal(t, "lookup", member)

	_, err = llm.ValidateNamespaceFunctionName("", "lookup")
	require.ErrorContains(t, err, "namespace is required")
	_, err = llm.ValidateNamespaceFunctionName("functions", "functions__")
	require.ErrorContains(t, err, "must use flattened name")
	_, err = llm.ValidateNamespaceFunctionName("functions", "other__lookup")
	require.ErrorContains(t, err, "must use flattened name")

	namespace, member, err := llm.SplitNamespaceFunctionName("functions__nested__lookup")
	require.NoError(t, err)
	require.Equal(t, "functions", namespace)
	require.Equal(t, "nested__lookup", member)

	_, _, err = llm.SplitNamespaceFunctionName("lookup")
	require.ErrorContains(t, err, "must use flattened name")
	_, _, err = llm.SplitNamespaceFunctionName("functions__")
	require.ErrorContains(t, err, "must use flattened name")
}

func TestToolChoiceUnmarshalJSONRejectsNullAllowedTools(t *testing.T) {
	auto := "auto"
	choice := llm.ToolChoice{
		ToolChoice:      &auto,
		NamedToolChoice: &llm.NamedToolChoice{Type: "custom", Function: llm.ToolFunction{Name: "stale"}},
	}
	before := choice

	err := json.Unmarshal([]byte(`{"mode":"auto","tools":null}`), &choice)
	require.ErrorContains(t, err, "tools must not be null")
	require.Equal(t, before, choice)
}

func TestToolChoiceUnmarshalJSONClearsPreviousVariant(t *testing.T) {
	auto := "auto"
	choice := llm.ToolChoice{
		ToolChoice: &auto, AllowedToolsSet: true,
		AllowedTools: []llm.ToolOption{{Type: "function", Name: "lookup"}},
	}

	require.NoError(t, json.Unmarshal([]byte(`"none"`), &choice))
	require.Equal(t, "none", *choice.ToolChoice)
	require.Nil(t, choice.NamedToolChoice)
	require.False(t, choice.AllowedToolsSet)
	require.Nil(t, choice.AllowedTools)

	require.NoError(t, json.Unmarshal([]byte(`{"type":"function","function":{"name":"lookup"}}`), &choice))
	require.Nil(t, choice.ToolChoice)
	require.NotNil(t, choice.NamedToolChoice)
	require.Equal(t, "lookup", choice.NamedToolChoice.Function.Name)
	require.False(t, choice.AllowedToolsSet)
	require.Nil(t, choice.AllowedTools)

	require.NoError(t, json.Unmarshal([]byte(`{"mode":"auto","tools":[]}`), &choice))
	require.Equal(t, "auto", *choice.ToolChoice)
	require.Nil(t, choice.NamedToolChoice)
	require.True(t, choice.AllowedToolsSet)
	require.NotNil(t, choice.AllowedTools)
	require.Empty(t, choice.AllowedTools)
}

func TestToolChoiceJSONVariantMatrix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, choice llm.ToolChoice)
	}{
		{
			name:  "auto mode",
			input: `"auto"`,
			validate: func(t *testing.T, choice llm.ToolChoice) {
				require.Equal(t, "auto", *choice.ToolChoice)
				require.Nil(t, choice.NamedToolChoice)
				require.False(t, choice.AllowedToolsSet)
			},
		},
		{
			name:  "named function",
			input: `{"type":"function","function":{"name":"lookup"}}`,
			validate: func(t *testing.T, choice llm.ToolChoice) {
				require.Nil(t, choice.ToolChoice)
				require.Equal(t, "function", choice.NamedToolChoice.Type)
				require.Equal(t, "lookup", choice.NamedToolChoice.Function.Name)
			},
		},
		{
			name:  "named custom primitive",
			input: `{"type":"custom","function":{"name":"apply_patch"}}`,
			validate: func(t *testing.T, choice llm.ToolChoice) {
				require.Equal(t, "custom", choice.NamedToolChoice.Type)
				require.Equal(t, "apply_patch", choice.NamedToolChoice.Function.Name)
			},
		},
		{
			name:  "named future client primitive",
			input: `{"type":"future_client_tool","function":{"name":"later_lookup"}}`,
			validate: func(t *testing.T, choice llm.ToolChoice) {
				require.Equal(t, "future_client_tool", choice.NamedToolChoice.Type)
				require.Equal(t, "later_lookup", choice.NamedToolChoice.Function.Name)
			},
		},
		{
			name: "allowed primitive matrix keeps order and identity",
			input: `{"mode":"required","tools":[
				{"type":"function","name":"same"},
				{"type":"custom","name":"same"},
				{"type":"namespace","name":"workspace"},
				{"type":"tool_search","name":"discover"},
				{"type":"future_client_tool","name":"later"},
				{"type":"future_server_tool","name":"hosted"}
			]}`,
			validate: func(t *testing.T, choice llm.ToolChoice) {
				require.Equal(t, "required", *choice.ToolChoice)
				require.True(t, choice.AllowedToolsSet)
				require.Equal(t, []llm.ToolOption{
					{Type: "function", Name: "same"},
					{Type: "custom", Name: "same"},
					{Type: "namespace", Name: "workspace"},
					{Type: "tool_search", Name: "discover"},
					{Type: "future_client_tool", Name: "later"},
					{Type: "future_server_tool", Name: "hosted"},
				}, choice.AllowedTools)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var decoded llm.ToolChoice
			require.NoError(t, json.Unmarshal([]byte(tt.input), &decoded))
			tt.validate(t, decoded)

			encoded, err := json.Marshal(decoded)
			require.NoError(t, err)
			require.JSONEq(t, tt.input, string(encoded))

			var roundTripped llm.ToolChoice
			require.NoError(t, json.Unmarshal(encoded, &roundTripped))
			require.Equal(t, decoded, roundTripped)
		})
	}
}

func TestToolChoiceUnmarshalJSONErrorDoesNotMutate(t *testing.T) {
	invalidInputs := []string{`{`, `1`, `true`, `[]`, `["auto"]`}
	for _, input := range invalidInputs {
		t.Run(input, func(t *testing.T) {
			auto := "auto"
			choice := llm.ToolChoice{
				ToolChoice:      &auto,
				NamedToolChoice: &llm.NamedToolChoice{Type: "custom", Function: llm.ToolFunction{Name: "stale"}},
				AllowedTools:    []llm.ToolOption{{Type: "function", Name: "lookup"}},
				AllowedToolsSet: true,
			}
			before := choice

			require.Error(t, json.Unmarshal([]byte(input), &choice))
			require.Equal(t, before, choice)
		})
	}
}

func FuzzToolChoiceJSONRoundTrip(f *testing.F) {
	seeds := []string{
		`"auto"`,
		`{"type":"function","function":{"name":"lookup"}}`,
		`{"type":"custom","function":{"name":"apply_patch"}}`,
		`{"type":"future_client_tool","function":{"name":"later"}}`,
		`{"mode":"auto","tools":[]}`,
		`{"mode":"required","tools":[{"type":"function","name":"lookup"},{"type":"custom","name":"apply_patch"}]}`,
		`null`,
		`{}`,
		`{`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		var decoded llm.ToolChoice
		if err := json.Unmarshal([]byte(input), &decoded); err != nil {
			return
		}

		encoded, err := json.Marshal(decoded)
		require.NoError(t, err)
		var roundTripped llm.ToolChoice
		require.NoError(t, json.Unmarshal(encoded, &roundTripped))
		reencoded, err := json.Marshal(roundTripped)
		require.NoError(t, err)
		require.JSONEq(t, string(encoded), string(reencoded))
	})
}
