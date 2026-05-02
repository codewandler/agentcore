package harness

import (
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/thread"
)

type SessionInfo struct {
	SessionID     string
	AgentName     string
	ThreadID      thread.ID
	BranchID      thread.BranchID
	ThreadBacked  bool
	ParamsSummary string
}

type SessionInfoPayload struct {
	Info SessionInfo
}

func (p SessionInfoPayload) Display(command.DisplayMode) (string, error) {
	info := p.Info
	var b strings.Builder
	b.WriteString("session:")
	writeSessionField(&b, "id", valueOrDash(info.SessionID))
	writeSessionField(&b, "agent", valueOrDash(info.AgentName))
	if info.ThreadBacked {
		writeSessionField(&b, "thread", string(info.ThreadID))
		writeSessionField(&b, "branch", string(info.BranchID))
	} else {
		writeSessionField(&b, "thread", "-")
	}
	if info.ParamsSummary != "" {
		writeSessionField(&b, "model", info.ParamsSummary)
	}
	return b.String(), nil
}

func writeSessionField(b *strings.Builder, name string, value string) {
	fmt.Fprintf(b, "\n%s: %s", name, value)
}

func valueOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
