package runtime

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/toolactivation"
	"github.com/stretchr/testify/require"
)

func TestWithToolActivationStoresToolManagementState(t *testing.T) {
	state := toolactivation.New(tool.New("one", "test", func(ctx tool.Ctx, p struct{}) (tool.Result, error) {
		return tool.Text("ok"), nil
	}))

	ctx := NewToolContext(context.Background(), WithToolActivation(state))

	require.Same(t, state, ctx.Extra()[toolactivation.ContextKey])
}

func TestWithToolSkillActivationStoresSkillState(t *testing.T) {
	state := &skill.ActivationState{}

	ctx := NewToolContext(context.Background(), WithToolSkillActivation(state))

	require.Same(t, state, ctx.Extra()[skill.ContextKey])
}
