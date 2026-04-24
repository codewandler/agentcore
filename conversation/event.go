package conversation

import "github.com/codewandler/llmadapter/unified"

type PayloadKind string

const (
	PayloadMessage    PayloadKind = "message"
	PayloadCompaction PayloadKind = "compaction"
	PayloadAnnotation PayloadKind = "annotation"
)

type Payload interface {
	Kind() PayloadKind
}

type MessageEvent struct {
	Message unified.Message `json:"message"`
}

func (MessageEvent) Kind() PayloadKind { return PayloadMessage }

type CompactionEvent struct {
	Summary  string   `json:"summary"`
	Replaces []NodeID `json:"replaces,omitempty"`
}

func (CompactionEvent) Kind() PayloadKind { return PayloadCompaction }

type AnnotationEvent struct {
	Text string         `json:"text,omitempty"`
	Meta map[string]any `json:"meta,omitempty"`
}

func (AnnotationEvent) Kind() PayloadKind { return PayloadAnnotation }
