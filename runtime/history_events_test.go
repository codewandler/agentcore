package runtime

import (
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestHistoryEventsPreserveMessagePhase(t *testing.T) {
	payload := conversation.AssistantTurnEvent{
		Message: unified.Message{
			Role:    unified.RoleAssistant,
			Phase:   unified.MessagePhaseCommentary,
			Content: []unified.ContentPart{unified.TextPart{Text: "working"}},
		},
		FinishReason: unified.FinishReasonStop,
	}
	wire, err := payloadToWire(payload)
	require.NoError(t, err)
	raw, err := json.Marshal(wire)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"phase":"commentary"`)

	roundTrip, err := unmarshalAssistantTurnPayload(raw)
	require.NoError(t, err)
	require.Equal(t, unified.MessagePhaseCommentary, roundTrip.Message.Phase)
}
