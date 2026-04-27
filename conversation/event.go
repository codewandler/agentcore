package conversation

import "github.com/codewandler/llmadapter/unified"

type Payload interface {
	conversationPayload()
}

type MessageEvent struct {
	Message unified.Message `json:"message"`
}

func (MessageEvent) conversationPayload() {}

type AssistantTurnEvent struct {
	Message       unified.Message        `json:"message"`
	FinishReason  unified.FinishReason   `json:"finish_reason,omitempty"`
	Usage         unified.Usage          `json:"usage,omitempty"`
	Continuations []ProviderContinuation `json:"continuations,omitempty"`
}

func (AssistantTurnEvent) conversationPayload() {}

type CompactionEvent struct {
	Summary  string   `json:"summary"`
	Replaces []NodeID `json:"replaces,omitempty"`
}

func (CompactionEvent) conversationPayload() {}

type AnnotationEvent struct {
	Text string         `json:"text,omitempty"`
	Meta map[string]any `json:"meta,omitempty"`
}

func (AnnotationEvent) conversationPayload() {}
