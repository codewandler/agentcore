package tool

import (
	"encoding/json"
	"testing"

	"github.com/codewandler/llmadapter/unified"
	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/require"
)

func TestStringSliceParam_Unmarshal_SingularString(t *testing.T) {
	var p struct {
		Paths StringSliceParam `json:"paths"`
	}
	input := `{"paths": "file1.go"}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string{"file1.go"}, []string(p.Paths))
}

func TestStringSliceParam_Unmarshal_ArrayOfStrings(t *testing.T) {
	var p struct {
		Paths StringSliceParam `json:"paths"`
	}
	input := `{"paths": ["file1.go", "file2.go"]}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string{"file1.go", "file2.go"}, []string(p.Paths))
}

func TestStringSliceParam_Unmarshal_EmptyArray(t *testing.T) {
	var p struct {
		Paths StringSliceParam `json:"paths"`
	}
	input := `{"paths": []}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string{}, []string(p.Paths))
}

func TestStringSliceParam_Unmarshal_Nil(t *testing.T) {
	var p struct {
		Paths StringSliceParam `json:"paths"`
	}
	input := `{}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string(nil), []string(p.Paths))
}

func TestStringSliceParam_JSONSchema(t *testing.T) {
	schema := StringSliceParam{}.JSONSchema()

	// Verify the schema has oneOf
	require.NotNil(t, schema.OneOf)
	require.Len(t, schema.OneOf, 2)

	// First option should be a string
	require.Equal(t, "string", schema.OneOf[0].Type)

	// Second option should be an array with string items
	require.Equal(t, "array", schema.OneOf[1].Type)
	require.NotNil(t, schema.OneOf[1].Items)
	require.Equal(t, "string", schema.OneOf[1].Items.Type)
}

func TestUnifiedToolsFrom(t *testing.T) {
	tl := New("lookup", "Look up a thing.", func(ctx Ctx, p struct {
		Query string `json:"query" jsonschema:"description=Search query,required"`
		Limit int    `json:"limit,omitempty" jsonschema:"description=Max results"`
	}) (Result, error) {
		return Text("ok"), nil
	})

	tools := UnifiedToolsFrom([]Tool{tl})

	require.Len(t, tools, 1)
	require.Equal(t, unified.ToolKindFunction, tools[0].Kind)
	require.Equal(t, "lookup", tools[0].Name)
	require.Equal(t, "Look up a thing.", tools[0].Description)
	require.NotEmpty(t, tools[0].InputSchema)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(tools[0].InputSchema, &schema))
	require.Equal(t, "object", schema["type"])

	props := schema["properties"].(map[string]any)
	require.Equal(t, "string", props["query"].(map[string]any)["type"])
	require.Equal(t, "integer", props["limit"].(map[string]any)["type"])
}

// Test that StringSliceParam is properly reflected by the jsonschema.Reflector
func TestStringSliceParam_InReflector(t *testing.T) {
	type TestParams struct {
		Paths StringSliceParam `json:"paths"`
	}

	reflector := jsonschema.Reflector{
		RequiredFromJSONSchemaTags: true,
		DoNotReference:             true,
	}

	schema := reflector.Reflect(&TestParams{})

	// Convert to JSON to inspect
	b, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	// The schema should contain oneOf for the paths field
	var result map[string]any
	err = json.Unmarshal(b, &result)
	require.NoError(t, err)

	// Navigate to the paths property
	props, ok := result["properties"].(map[string]any)
	require.True(t, ok)

	paths, ok := props["paths"].(map[string]any)
	require.True(t, ok)

	// Should have oneOf
	oneOf, ok := paths["oneOf"].([]any)
	require.True(t, ok, "Expected paths to have oneOf, got: %s", string(b))
	require.Len(t, oneOf, 2)
}
