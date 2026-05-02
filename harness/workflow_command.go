package harness

import (
	"context"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/workflow"
)

type WorkflowCommandHandler struct {
	Session *Session
}

func isWorkflowCommand(input string) bool {
	input = strings.TrimSpace(input)
	return input == "/workflow" || strings.HasPrefix(input, "/workflow ")
}

func (h WorkflowCommandHandler) HandleInput(ctx context.Context, input string) (command.Result, error) {
	_, params, err := command.Parse(input)
	if err != nil {
		return command.Result{}, err
	}
	return h.Handle(ctx, params)
}

func (h WorkflowCommandHandler) Handle(ctx context.Context, params command.Params) (command.Result, error) {
	if len(params.Args) == 0 {
		return command.Text(workflowCommandUsage()), nil
	}
	switch params.Args[0] {
	case "list":
		if len(params.Args) != 1 {
			return command.Text("usage: /workflow list"), nil
		}
		return h.workflowList(), nil
	case "show":
		if len(params.Args) != 2 {
			return command.Text("usage: /workflow show <name>"), nil
		}
		return h.workflowShow(params.Args[1]), nil
	case "start":
		if len(params.Args) < 2 {
			return command.Text("usage: /workflow start <name> [input]"), nil
		}
		return h.workflowStart(ctx, params.Args[1], strings.Join(params.Args[2:], " "))
	case "runs":
		if len(params.Args) != 1 {
			return command.Text("usage: /workflow runs"), nil
		}
		return h.workflowRuns(ctx)
	case "run":
		if len(params.Args) != 2 {
			return command.Text("usage: /workflow run <run-id>"), nil
		}
		return h.workflowRun(ctx, workflow.RunID(params.Args[1]))
	default:
		return command.Text(workflowCommandUsage()), nil
	}
}

func workflowCommandUsage() string {
	return "usage: /workflow <list|show|start|runs|run>\n  /workflow list\n  /workflow show <name>\n  /workflow start <name> [input]\n  /workflow runs\n  /workflow run <run-id>"
}

func (h WorkflowCommandHandler) workflowList() command.Result {
	s := h.Session
	if s == nil || s.App == nil {
		return command.Display(WorkflowListPayload{})
	}
	return command.Display(WorkflowListPayload{Definitions: s.App.Workflows()})
}

func (h WorkflowCommandHandler) workflowShow(name string) command.Result {
	s := h.Session
	if s == nil || s.App == nil {
		return command.Text(fmt.Sprintf("workflow %q not found", name))
	}
	def, ok := s.App.Workflow(name)
	if !ok {
		return command.Text(fmt.Sprintf("workflow %q not found", name))
	}
	return command.Display(WorkflowDefinitionPayload{Definition: def})
}

func (h WorkflowCommandHandler) workflowStart(ctx context.Context, workflowName string, input string) (command.Result, error) {
	s := h.Session
	if s == nil || s.App == nil {
		return command.Result{}, fmt.Errorf("harness: app is required")
	}
	if _, ok := s.App.Workflow(workflowName); !ok {
		return command.Text(fmt.Sprintf("workflow %q not found", workflowName)), nil
	}
	runID := workflow.NewRunID()
	result := s.ExecuteWorkflow(ctx, workflowName, input, app.WithWorkflowRunID(runID))
	if result.Error != nil {
		return command.Display(WorkflowStartPayload{WorkflowName: workflowName, RunID: runID, Status: workflow.RunFailed, Error: result.Error.Error()}), nil
	}
	data := result.Data
	if wfResult, ok := data.(workflow.Result); ok {
		data = wfResult.Data
	}
	return command.Display(WorkflowStartPayload{WorkflowName: workflowName, RunID: runID, Status: workflow.RunSucceeded, Output: data}), nil
}

func (h WorkflowCommandHandler) workflowRun(ctx context.Context, runID workflow.RunID) (command.Result, error) {
	s := h.Session
	state, ok, err := s.WorkflowRunState(ctx, runID)
	if err != nil {
		return command.Result{}, err
	}
	if !ok {
		if _, hasStore := s.WorkflowRunStore(); !hasStore {
			return command.Text("workflow runs require a thread-backed session"), nil
		}
		return command.Text(fmt.Sprintf("workflow run %q not found", runID)), nil
	}
	return command.Display(WorkflowRunPayload{State: state}), nil
}

func (h WorkflowCommandHandler) workflowRuns(ctx context.Context) (command.Result, error) {
	s := h.Session
	summaries, ok, err := s.WorkflowRuns(ctx)
	if err != nil {
		return command.Result{}, err
	}
	if !ok {
		return command.Text("workflow runs require a thread-backed session"), nil
	}
	return command.Display(WorkflowRunsPayload{Summaries: summaries}), nil
}
