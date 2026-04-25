package runnertest

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/codewandler/llmadapter/unified"
)

type Client struct {
	requests []unified.Request
	streams  [][]unified.Event
}

func NewClient(streams ...[]unified.Event) *Client {
	return &Client{streams: append([][]unified.Event(nil), streams...)}
}

func (c *Client) Request(_ context.Context, req unified.Request) (<-chan unified.Event, error) {
	c.requests = append(c.requests, req)
	if len(c.streams) == 0 {
		out := make(chan unified.Event)
		close(out)
		return out, nil
	}
	events := c.streams[0]
	c.streams = c.streams[1:]
	out := make(chan unified.Event, len(events))
	for _, event := range events {
		out <- event
	}
	close(out)
	return out, nil
}

func (c *Client) Requests() []unified.Request {
	return append([]unified.Request(nil), c.requests...)
}

func (c *Client) RequestAt(index int) unified.Request {
	if index < 0 || index >= len(c.requests) {
		return unified.Request{}
	}
	return c.requests[index]
}

func TextStream(text string, messageID ...string) []unified.Event {
	id := first(messageID)
	return []unified.Event{
		unified.TextDeltaEvent{Text: text},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: id},
	}
}

func ReasoningTextStream(reasoning string, signature string, text string, messageID ...string) []unified.Event {
	id := first(messageID)
	return []unified.Event{
		unified.ContentBlockStartEvent{Index: 0, Kind: unified.ContentKindReasoning},
		unified.ReasoningDeltaEvent{Index: 0, Text: reasoning, Signature: signature},
		unified.ContentBlockDoneEvent{Index: 0, Kind: unified.ContentKindReasoning},
		unified.TextDeltaEvent{Text: text},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: id},
	}
}

func ToolCallStream(messageID string, calls ...unified.ToolCall) []unified.Event {
	out := make([]unified.Event, 0, len(calls)*2+1)
	for _, call := range calls {
		out = append(out,
			unified.ToolCallStartEvent{Index: call.Index, ID: call.ID, Name: call.Name},
			unified.ToolCallDoneEvent{Index: call.Index, ID: call.ID, Name: call.Name, Args: call.Arguments},
		)
	}
	out = append(out, unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall, MessageID: messageID})
	return out
}

func ToolCall(name string, id string, index int, args string) unified.ToolCall {
	return unified.ToolCall{
		ID:        id,
		Name:      name,
		Index:     index,
		Arguments: json.RawMessage(args),
	}
}

func ErrorStream(err error) []unified.Event {
	if err == nil {
		err = errors.New("stream error")
	}
	return []unified.Event{unified.ErrorEvent{Err: err}}
}

func IncompleteTextStream(text string) []unified.Event {
	return []unified.Event{unified.TextDeltaEvent{Text: text}}
}

func Route(providerName string, api string, family string, publicModel string, nativeModel string) unified.RouteEvent {
	return unified.RouteEvent{
		ProviderName: providerName,
		TargetAPI:    api,
		TargetFamily: family,
		PublicModel:  publicModel,
		NativeModel:  nativeModel,
	}
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
