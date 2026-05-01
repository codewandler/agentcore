package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/codewandler/agentsdk/action"
	"github.com/stretchr/testify/require"
)

func TestExecutorRunsWorkflowOverActionRefs(t *testing.T) {
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(
		action.New(action.Spec{Name: "upper"}, func(_ action.Ctx, input any) action.Result {
			require.Equal(t, "hello", input)
			return action.Result{Data: "HELLO"}
		}),
		action.New(action.Spec{Name: "suffix"}, func(_ action.Ctx, input any) action.Result {
			require.Equal(t, "HELLO", input)
			return action.Result{Data: input.(string) + "!"}
		}),
	))

	def := Definition{Name: "shout", Steps: []Step{
		{ID: "upper", Action: ActionRef{Name: "upper"}},
		{ID: "suffix", Action: ActionRef{Name: "suffix"}, DependsOn: []string{"upper"}},
	}}

	result := Executor{Resolver: RegistryResolver{Registry: reg}}.Execute(context.Background(), def, "hello")
	require.NoError(t, result.Error)
	wfResult := result.Data.(Result)
	require.Equal(t, "HELLO!", wfResult.Data)
	require.Equal(t, "HELLO", wfResult.StepResults["upper"].Data)
}

func TestExecutorPassesMultipleDependencyOutputs(t *testing.T) {
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(
		action.New(action.Spec{Name: "a"}, func(action.Ctx, any) action.Result { return action.Result{Data: "A"} }),
		action.New(action.Spec{Name: "b"}, func(action.Ctx, any) action.Result { return action.Result{Data: "B"} }),
		action.New(action.Spec{Name: "join"}, func(_ action.Ctx, input any) action.Result {
			deps := input.(map[string]any)
			return action.Result{Data: deps["a"].(string) + deps["b"].(string)}
		}),
	))
	def := Definition{Name: "join", Steps: []Step{
		{ID: "a", Action: ActionRef{Name: "a"}},
		{ID: "b", Action: ActionRef{Name: "b"}},
		{ID: "join", Action: ActionRef{Name: "join"}, DependsOn: []string{"a", "b"}},
	}}

	result := Executor{Resolver: RegistryResolver{Registry: reg}}.Execute(context.Background(), def, nil)
	require.NoError(t, result.Error)
	require.Equal(t, "AB", result.Data.(Result).Data)
}

func TestExecutorStopsOnStepError(t *testing.T) {
	boom := errors.New("boom")
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(action.New(action.Spec{Name: "fail"}, func(action.Ctx, any) action.Result {
		return action.Result{Error: boom}
	})))

	def := Definition{Name: "failflow", Steps: []Step{{ID: "fail", Action: ActionRef{Name: "fail"}}}}
	result := Executor{Resolver: RegistryResolver{Registry: reg}}.Execute(context.Background(), def, nil)
	require.ErrorIs(t, result.Error, boom)
	wfResult := result.Data.(Result)
	require.Contains(t, wfResult.StepResults, "fail")
}

func TestExecutorRejectsCyclesAndUnknownDeps(t *testing.T) {
	exec := Executor{Resolver: ResolverFunc(func(action.Ctx, ActionRef) (action.Action, bool) { return nil, false })}

	cycle := Definition{Name: "cycle", Steps: []Step{
		{ID: "a", Action: ActionRef{Name: "a"}, DependsOn: []string{"b"}},
		{ID: "b", Action: ActionRef{Name: "b"}, DependsOn: []string{"a"}},
	}}
	require.ErrorContains(t, exec.Execute(context.Background(), cycle, nil).Error, "cycle")

	unknown := Definition{Name: "unknown", Steps: []Step{{ID: "a", Action: ActionRef{Name: "a"}, DependsOn: []string{"missing"}}}}
	require.ErrorContains(t, exec.Execute(context.Background(), unknown, nil).Error, "unknown step")
}

func TestWorkflowActionExposesDefinition(t *testing.T) {
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(action.New(action.Spec{Name: "echo"}, func(_ action.Ctx, input any) action.Result {
		return action.Result{Data: input}
	})))
	wa := WorkflowAction{
		Definition: Definition{Name: "echo_flow", Description: "echo workflow", Steps: []Step{{ID: "echo", Action: ActionRef{Name: "echo"}}}},
		Executor:   Executor{Resolver: RegistryResolver{Registry: reg}},
	}

	require.Equal(t, "echo_flow", wa.Spec().Name)
	result := wa.Execute(context.Background(), "hi")
	require.NoError(t, result.Error)
	require.Equal(t, "hi", result.Data.(Result).Data)
}
