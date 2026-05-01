package workflow

// ValueRef describes a workflow value in a form that can later become durable.
// Runtime action dataflow remains Go-native `any`; workflow events and
// projected run state store outputs as ValueRef.
type ValueRef struct {
	ID          string `json:"id,omitempty"`
	MediaType   string `json:"media_type,omitempty"`
	Inline      any    `json:"inline,omitempty"`
	ExternalURI string `json:"external_uri,omitempty"`
	Redacted    bool   `json:"redacted,omitempty"`
}

// InlineValue stores value directly in the workflow event/state payload.
func InlineValue(value any) ValueRef {
	if value == nil {
		return ValueRef{}
	}
	return ValueRef{Inline: value}
}

// ExternalValue references a value stored outside the workflow event/state
// payload.
func ExternalValue(uri, mediaType string) ValueRef {
	return ValueRef{ExternalURI: uri, MediaType: mediaType}
}

// RedactedValue records that a value exists but is intentionally omitted.
func RedactedValue(id string) ValueRef {
	return ValueRef{ID: id, Redacted: true}
}
