package runtime

import (
	"encoding/json"
	"fmt"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/llmadapter/unified"
)

const (
	eventConversationUserMessage      thread.EventKind = "conversation.user_message"
	eventConversationAssistantMessage thread.EventKind = "conversation.assistant_message"
	eventConversationToolResult       thread.EventKind = "conversation.tool_result"
	eventConversationMessage          thread.EventKind = "conversation.message"
	eventConversationCompaction       thread.EventKind = "conversation.compaction"
	eventConversationAnnotation       thread.EventKind = "conversation.annotation"
)

func EventDefinitions() []thread.EventDefinition {
	return []thread.EventDefinition{
		thread.DefineEvent[wireMessageEvent](eventConversationUserMessage),
		thread.DefineEvent[wireAssistantTurnEvent](eventConversationAssistantMessage),
		thread.DefineEvent[wireMessageEvent](eventConversationToolResult),
		thread.DefineEvent[wireMessageEvent](eventConversationMessage),
		thread.DefineEvent[conversation.CompactionEvent](eventConversationCompaction),
		thread.DefineEvent[conversation.AnnotationEvent](eventConversationAnnotation),
		thread.DefineEvent[contextFragmentRecorded](EventContextFragmentRecorded),
		thread.DefineEvent[contextFragmentRemovedRecorded](EventContextFragmentRemoved),
		thread.DefineEvent[contextSnapshotRecorded](EventContextSnapshotRecorded),
		thread.DefineEvent[contextRenderCommitted](EventContextRenderCommitted),
		thread.DefineEvent[skill.SkillActivatedEvent](skill.EventSkillActivated),
		thread.DefineEvent[skill.SkillReferenceActivatedEvent](skill.EventSkillReferenceActivated),
	}
}

func threadEventFromPayload(payload conversation.Payload, event thread.Event) (thread.Event, error) {
	kind, raw, err := marshalThreadPayload(payload)
	if err != nil {
		return thread.Event{}, err
	}
	event.Kind = kind
	event.Payload = raw
	return event, nil
}

func payloadFromThreadEvent(event thread.Event) (conversation.Payload, bool, error) {
	switch event.Kind {
	case eventConversationUserMessage, eventConversationToolResult, eventConversationMessage:
		payload, err := unmarshalMessagePayload(event.Payload)
		if err != nil {
			return nil, false, err
		}
		return payload, true, nil
	case eventConversationAssistantMessage:
		payload, err := unmarshalAssistantTurnPayload(event.Payload)
		if err != nil {
			if messagePayload, messageErr := unmarshalMessagePayload(event.Payload); messageErr == nil {
				return messagePayload, true, nil
			}
			return nil, false, err
		}
		return payload, true, nil
	case eventConversationCompaction:
		payload, err := unmarshalCompactionPayload(event.Payload)
		if err != nil {
			return nil, false, err
		}
		return payload, true, nil
	case eventConversationAnnotation:
		payload, err := unmarshalAnnotationPayload(event.Payload)
		if err != nil {
			return nil, false, err
		}
		return payload, true, nil
	default:
		return nil, false, nil
	}
}

func marshalThreadPayload(payload conversation.Payload) (thread.EventKind, json.RawMessage, error) {
	switch payload := payload.(type) {
	case conversation.MessageEvent:
		return marshalMessagePayload(payload)
	case *conversation.MessageEvent:
		if payload == nil {
			return "", nil, fmt.Errorf("runtime: message payload is nil")
		}
		return marshalMessagePayload(*payload)
	case conversation.AssistantTurnEvent:
		wire, err := payloadToWire(payload)
		if err != nil {
			return "", nil, err
		}
		raw, err := json.Marshal(wire)
		return eventConversationAssistantMessage, raw, err
	case *conversation.AssistantTurnEvent:
		if payload == nil {
			return "", nil, fmt.Errorf("runtime: assistant turn payload is nil")
		}
		wire, err := payloadToWire(payload)
		if err != nil {
			return "", nil, err
		}
		raw, err := json.Marshal(wire)
		return eventConversationAssistantMessage, raw, err
	case conversation.CompactionEvent:
		raw, err := json.Marshal(payload)
		return eventConversationCompaction, raw, err
	case *conversation.CompactionEvent:
		if payload == nil {
			return "", nil, fmt.Errorf("runtime: compaction payload is nil")
		}
		raw, err := json.Marshal(payload)
		return eventConversationCompaction, raw, err
	case conversation.AnnotationEvent:
		raw, err := json.Marshal(payload)
		return eventConversationAnnotation, raw, err
	case *conversation.AnnotationEvent:
		if payload == nil {
			return "", nil, fmt.Errorf("runtime: annotation payload is nil")
		}
		raw, err := json.Marshal(payload)
		return eventConversationAnnotation, raw, err
	default:
		return "", nil, fmt.Errorf("runtime: unsupported history payload %T", payload)
	}
}

func marshalMessagePayload(payload conversation.MessageEvent) (thread.EventKind, json.RawMessage, error) {
	wire, err := payloadToWire(payload)
	if err != nil {
		return "", nil, err
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return "", nil, err
	}
	switch payload.Message.Role {
	case unified.RoleUser:
		return eventConversationUserMessage, raw, nil
	case unified.RoleTool:
		return eventConversationToolResult, raw, nil
	case unified.RoleAssistant:
		return eventConversationAssistantMessage, raw, nil
	default:
		return eventConversationMessage, raw, nil
	}
}

type wireMessageEvent struct {
	Message wireMessage `json:"message"`
}

type wireAssistantTurnEvent struct {
	Message       wireMessage                         `json:"message"`
	FinishReason  unified.FinishReason                `json:"finish_reason,omitempty"`
	Usage         unified.Usage                       `json:"usage,omitempty"`
	Continuations []conversation.ProviderContinuation `json:"continuations,omitempty"`
}

type wireMessage struct {
	Role        unified.Role       `json:"role"`
	ID          string             `json:"id,omitempty"`
	Name        string             `json:"name,omitempty"`
	Content     []wireContentPart  `json:"content,omitempty"`
	ToolCalls   []unified.ToolCall `json:"tool_calls,omitempty"`
	ToolResults []wireToolResult   `json:"tool_results,omitempty"`
	Meta        map[string]any     `json:"meta,omitempty"`
}

type wireToolResult struct {
	ToolCallID string            `json:"tool_call_id"`
	Name       string            `json:"name,omitempty"`
	Content    []wireContentPart `json:"content,omitempty"`
	IsError    bool              `json:"is_error,omitempty"`
}

type wireContentPart struct {
	Type         unified.ContentKind   `json:"type"`
	Text         string                `json:"text,omitempty"`
	Signature    string                `json:"signature,omitempty"`
	CacheControl *unified.CacheControl `json:"cache_control,omitempty"`
	Source       *unified.BlobSource   `json:"source,omitempty"`
	Alt          string                `json:"alt,omitempty"`
	Filename     string                `json:"filename,omitempty"`
	MIMEType     string                `json:"mime_type,omitempty"`
}

func unmarshalMessagePayload(b []byte) (conversation.MessageEvent, error) {
	var payload wireMessageEvent
	if err := json.Unmarshal(b, &payload); err != nil {
		return conversation.MessageEvent{}, err
	}
	return conversation.MessageEvent{Message: payload.Message.unified()}, nil
}

func unmarshalAssistantTurnPayload(b []byte) (conversation.AssistantTurnEvent, error) {
	var payload wireAssistantTurnEvent
	if err := json.Unmarshal(b, &payload); err != nil {
		return conversation.AssistantTurnEvent{}, err
	}
	return conversation.AssistantTurnEvent{
		Message:       payload.Message.unified(),
		FinishReason:  payload.FinishReason,
		Usage:         payload.Usage,
		Continuations: payload.Continuations,
	}, nil
}

func unmarshalCompactionPayload(b []byte) (conversation.CompactionEvent, error) {
	var payload conversation.CompactionEvent
	if err := json.Unmarshal(b, &payload); err != nil {
		return conversation.CompactionEvent{}, err
	}
	return payload, nil
}

func unmarshalAnnotationPayload(b []byte) (conversation.AnnotationEvent, error) {
	var payload conversation.AnnotationEvent
	if err := json.Unmarshal(b, &payload); err != nil {
		return conversation.AnnotationEvent{}, err
	}
	return payload, nil
}

func payloadToWire(payload conversation.Payload) (any, error) {
	switch payload := payload.(type) {
	case conversation.MessageEvent:
		msg, err := messageToWire(payload.Message)
		if err != nil {
			return nil, err
		}
		return wireMessageEvent{Message: msg}, nil
	case *conversation.MessageEvent:
		if payload == nil {
			return nil, fmt.Errorf("runtime: message payload is nil")
		}
		msg, err := messageToWire(payload.Message)
		if err != nil {
			return nil, err
		}
		return wireMessageEvent{Message: msg}, nil
	case conversation.AssistantTurnEvent:
		msg, err := messageToWire(payload.Message)
		if err != nil {
			return nil, err
		}
		return wireAssistantTurnEvent{
			Message:       msg,
			FinishReason:  payload.FinishReason,
			Usage:         payload.Usage,
			Continuations: payload.Continuations,
		}, nil
	case *conversation.AssistantTurnEvent:
		if payload == nil {
			return nil, fmt.Errorf("runtime: assistant turn payload is nil")
		}
		msg, err := messageToWire(payload.Message)
		if err != nil {
			return nil, err
		}
		return wireAssistantTurnEvent{
			Message:       msg,
			FinishReason:  payload.FinishReason,
			Usage:         payload.Usage,
			Continuations: payload.Continuations,
		}, nil
	default:
		return payload, nil
	}
}

func messageToWire(msg unified.Message) (wireMessage, error) {
	content, err := contentPartsToWire(msg.Content)
	if err != nil {
		return wireMessage{}, err
	}
	toolResults := make([]wireToolResult, 0, len(msg.ToolResults))
	for _, result := range msg.ToolResults {
		resultContent, err := contentPartsToWire(result.Content)
		if err != nil {
			return wireMessage{}, err
		}
		toolResults = append(toolResults, wireToolResult{
			ToolCallID: result.ToolCallID,
			Name:       result.Name,
			Content:    resultContent,
			IsError:    result.IsError,
		})
	}
	return wireMessage{
		Role:        msg.Role,
		ID:          msg.ID,
		Name:        msg.Name,
		Content:     content,
		ToolCalls:   append([]unified.ToolCall(nil), msg.ToolCalls...),
		ToolResults: toolResults,
		Meta:        msg.Meta,
	}, nil
}

func (msg wireMessage) unified() unified.Message {
	toolResults := make([]unified.ToolResult, 0, len(msg.ToolResults))
	for _, result := range msg.ToolResults {
		toolResults = append(toolResults, unified.ToolResult{
			ToolCallID: result.ToolCallID,
			Name:       result.Name,
			Content:    contentPartsFromWire(result.Content),
			IsError:    result.IsError,
		})
	}
	return unified.Message{
		Role:        msg.Role,
		ID:          msg.ID,
		Name:        msg.Name,
		Content:     contentPartsFromWire(msg.Content),
		ToolCalls:   append([]unified.ToolCall(nil), msg.ToolCalls...),
		ToolResults: toolResults,
		Meta:        msg.Meta,
	}
}

func contentPartsToWire(parts []unified.ContentPart) ([]wireContentPart, error) {
	out := make([]wireContentPart, 0, len(parts))
	for _, part := range parts {
		wire, err := contentPartToWire(part)
		if err != nil {
			return nil, err
		}
		out = append(out, wire)
	}
	return out, nil
}

func contentPartToWire(part unified.ContentPart) (wireContentPart, error) {
	switch part := part.(type) {
	case unified.TextPart:
		return wireContentPart{Type: unified.ContentKindText, Text: part.Text, CacheControl: part.CacheControl}, nil
	case *unified.TextPart:
		if part == nil {
			return wireContentPart{}, fmt.Errorf("runtime: text content part is nil")
		}
		return wireContentPart{Type: unified.ContentKindText, Text: part.Text, CacheControl: part.CacheControl}, nil
	case unified.ImagePart:
		return wireContentPart{Type: unified.ContentKindImage, Source: &part.Source, Alt: part.Alt}, nil
	case unified.AudioPart:
		return wireContentPart{Type: unified.ContentKindAudio, Source: &part.Source}, nil
	case unified.VideoPart:
		return wireContentPart{Type: unified.ContentKindVideo, Source: &part.Source}, nil
	case unified.FilePart:
		return wireContentPart{Type: unified.ContentKindFile, Source: &part.Source, Filename: part.Filename, MIMEType: part.MIMEType}, nil
	case unified.ReasoningPart:
		return wireContentPart{Type: unified.ContentKindReasoning, Text: part.Text, Signature: part.Signature}, nil
	case unified.RefusalPart:
		return wireContentPart{Type: unified.ContentKindRefusal, Text: part.Text}, nil
	default:
		return wireContentPart{}, fmt.Errorf("runtime: unsupported content part type %T", part)
	}
}

func contentPartsFromWire(parts []wireContentPart) []unified.ContentPart {
	out := make([]unified.ContentPart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case unified.ContentKindText, "":
			out = append(out, unified.TextPart{Text: part.Text, CacheControl: part.CacheControl})
		case unified.ContentKindImage:
			out = append(out, unified.ImagePart{Source: derefBlobSource(part.Source), Alt: part.Alt})
		case unified.ContentKindAudio:
			out = append(out, unified.AudioPart{Source: derefBlobSource(part.Source)})
		case unified.ContentKindVideo:
			out = append(out, unified.VideoPart{Source: derefBlobSource(part.Source)})
		case unified.ContentKindFile:
			out = append(out, unified.FilePart{Source: derefBlobSource(part.Source), Filename: part.Filename, MIMEType: part.MIMEType})
		case unified.ContentKindReasoning:
			out = append(out, unified.ReasoningPart{Text: part.Text, Signature: part.Signature})
		case unified.ContentKindRefusal:
			out = append(out, unified.RefusalPart{Text: part.Text})
		}
	}
	return out
}

func derefBlobSource(source *unified.BlobSource) unified.BlobSource {
	if source == nil {
		return unified.BlobSource{}
	}
	return *source
}
