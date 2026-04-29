package toolmw

import (
	"testing"

	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/cmdrisk"
	"github.com/stretchr/testify/require"
)

func TestCmdRiskAssessor_ReusesPrecomputed(t *testing.T) {
	precomputed := &cmdrisk.Assessment{
		Command:    "rm -rf /",
		Confidence: cmdrisk.ConfidenceHigh,
		Decision: cmdrisk.Decision{
			Action:    cmdrisk.ActionReject,
			Reasons:   []string{"destructive"},
			Rationale: "deletes entire filesystem",
		},
		RiskDimensions: []cmdrisk.RiskDimension{
			{Name: "destructiveness", Severity: 10, Reason: "recursive delete from root"},
		},
		Behaviors: []string{"filesystem_delete"},
	}

	assessor := NewCmdRiskAssessor(nil) // analyzer not needed when Extra is set
	intent := tool.Intent{
		Tool:      "bash",
		ToolClass: "command_execution",
		Extra:     precomputed,
	}

	assessment, err := assessor.Assess(riskCtx(), intent)
	require.NoError(t, err)
	require.Equal(t, ActionReject, assessment.Decision.Action)
	require.Equal(t, "deletes entire filesystem", assessment.Decision.Rationale)
	require.Len(t, assessment.Dimensions, 1)
	require.Equal(t, 10, assessment.Dimensions[0].Severity)
	require.Equal(t, "high", assessment.Confidence)
}

func TestCmdRiskAssessor_FallbackWithoutExtra(t *testing.T) {
	assessor := NewCmdRiskAssessor(nil)
	intent := tool.Intent{
		Tool:      "bash",
		ToolClass: "command_execution",
		// No Extra — no pre-computed assessment.
	}

	assessment, err := assessor.Assess(riskCtx(), intent)
	require.NoError(t, err)
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
	require.Contains(t, assessment.Decision.Reasons, "opaque_command")
}

func TestCmdRiskAssessor_AcceptsIntent(t *testing.T) {
	assessor := NewCmdRiskAssessor(nil)

	// With Extra → accepts.
	require.True(t, assessor.AcceptsIntent(tool.Intent{
		Extra: &cmdrisk.Assessment{},
	}))

	// command_execution class → accepts.
	require.True(t, assessor.AcceptsIntent(tool.Intent{
		ToolClass: "command_execution",
	}))

	// Other class → rejects.
	require.False(t, assessor.AcceptsIntent(tool.Intent{
		ToolClass: "filesystem_read",
	}))
}

func TestCmdRiskAssessor_MapsAllActions(t *testing.T) {
	tests := []struct {
		action cmdrisk.Action
		want   Action
	}{
		{cmdrisk.ActionAllow, ActionAllow},
		{cmdrisk.ActionRequiresApproval, ActionRequiresApproval},
		{cmdrisk.ActionReject, ActionReject},
		{cmdrisk.ActionError, ActionReject},
	}

	assessor := NewCmdRiskAssessor(nil)
	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			assessment, err := assessor.Assess(riskCtx(), tool.Intent{
				Extra: &cmdrisk.Assessment{
					Decision: cmdrisk.Decision{Action: tt.action},
				},
			})
			require.NoError(t, err)
			require.Equal(t, tt.want, assessment.Decision.Action)
		})
	}
}

func TestCmdRiskAssessor_WithRealAnalyzer(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})

	// Use the real analyzer to assess a command.
	assessment, err := analyzer.Assess(t.Context(), cmdrisk.Request{
		Command: "ls -la",
		Context: cmdrisk.Context{
			Environment: cmdrisk.EnvironmentDeveloperWorkstation,
			Interactive: true,
			Trust: cmdrisk.TrustContext{
				CommandOrigin: cmdrisk.CommandOriginMachineGenerated,
			},
		},
	})
	require.NoError(t, err)

	// Map through our assessor — verify the bridge works end-to-end.
	assessor := NewCmdRiskAssessor(analyzer)
	result, err := assessor.Assess(riskCtx(), tool.Intent{
		Tool:      "bash",
		ToolClass: "command_execution",
		Extra:     &assessment,
	})
	require.NoError(t, err)
	// The action should be one of our valid actions (mapped from cmdrisk).
	require.Contains(t, []Action{ActionAllow, ActionRequiresApproval, ActionReject}, result.Decision.Action)
	// Confidence should be mapped from cmdrisk.
	require.NotEmpty(t, result.Confidence)
}
