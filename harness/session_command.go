package harness

import (
	"context"

	"github.com/codewandler/agentsdk/command"
)

type SessionCommandHandler struct {
	Session *Session
}

func NewSessionCommand(session *Session) (*command.Tree, error) {
	h := SessionCommandHandler{Session: session}
	tree := command.NewTree(command.Spec{Name: "session", Description: "Inspect the active session"})
	if _, err := tree.AddSub(command.Spec{Name: "info", Description: "Show session metadata"}, h.sessionInfoCommand); err != nil {
		return nil, err
	}
	return tree, nil
}

func (h SessionCommandHandler) sessionInfoCommand(context.Context, command.Invocation) (command.Result, error) {
	return command.Display(SessionInfoPayload{Info: h.Session.Info()}), nil
}
