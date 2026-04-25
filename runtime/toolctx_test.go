package runtime

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/activation"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/toolmgmt"
	"github.com/stretchr/testify/require"
)

func TestWithToolActivationStoresToolManagementState(t *testing.T) {
	state := activation.New(tool.New("one", "test", func(ctx tool.Ctx, p struct{}) (tool.Result, error) {
		return tool.Text("ok"), nil
	}))

	ctx := NewToolContext(context.Background(), WithToolActivation(state))

	require.Same(t, state, ctx.Extra()[toolmgmt.KeyActivationState])
}
