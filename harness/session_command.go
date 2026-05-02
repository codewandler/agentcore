package harness

import (
	"context"
	"strings"

	"github.com/codewandler/agentsdk/command"
)

type SessionCommandHandler struct {
	Session *Session
}

func isSessionCommand(input string) bool {
	input = strings.TrimSpace(input)
	return input == "/session" || strings.HasPrefix(input, "/session ")
}

func (h SessionCommandHandler) HandleInput(ctx context.Context, input string) (command.Result, error) {
	_, params, err := command.Parse(input)
	if err != nil {
		return command.Result{}, err
	}
	return h.Handle(ctx, params)
}

func (h SessionCommandHandler) Handle(_ context.Context, params command.Params) (command.Result, error) {
	if len(params.Args) == 0 || (len(params.Args) == 1 && params.Args[0] == "info") {
		return command.Display(SessionInfoPayload{Info: h.Session.Info()}), nil
	}
	return command.Text(sessionCommandUsage()), nil
}

func sessionCommandUsage() string {
	return "usage: /session [info]"
}
