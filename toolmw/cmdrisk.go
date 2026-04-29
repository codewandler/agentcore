package toolmw

import (
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/cmdrisk"
)

// CmdRiskAssessor bridges cmdrisk for bash tool assessment.
// It reuses a pre-computed cmdrisk.Assessment stored in Intent.Extra
// (placed there by bash's DeclareIntent), avoiding double analysis.
type CmdRiskAssessor struct {
	Analyzer *cmdrisk.Analyzer
}

// NewCmdRiskAssessor creates a CmdRiskAssessor with the given analyzer.
func NewCmdRiskAssessor(analyzer *cmdrisk.Analyzer) *CmdRiskAssessor {
	return &CmdRiskAssessor{Analyzer: analyzer}
}

// AcceptsIntent returns true for intents that carry a cmdrisk.Assessment
// in Extra, or for command_execution tools.
func (a *CmdRiskAssessor) AcceptsIntent(intent tool.Intent) bool {
	if _, ok := intent.Extra.(*cmdrisk.Assessment); ok {
		return true
	}
	return intent.ToolClass == "command_execution"
}

func (a *CmdRiskAssessor) Assess(ctx tool.Ctx, intent tool.Intent) (Assessment, error) {
	// Reuse pre-computed assessment if available (from DeclareIntent).
	if ca, ok := intent.Extra.(*cmdrisk.Assessment); ok {
		return mapCmdRiskAssessment(*ca), nil
	}

	// Fallback: opaque command_execution without cmdrisk data.
	return Assessment{
		Decision:   Decision{Action: ActionRequiresApproval, Reasons: []string{"opaque_command"}, Rationale: "command intent could not be determined"},
		Confidence: string(cmdrisk.ConfidenceLow),
	}, nil
}

func mapCmdRiskAssessment(ca cmdrisk.Assessment) Assessment {
	action := ActionAllow
	switch ca.Decision.Action {
	case cmdrisk.ActionRequiresApproval:
		action = ActionRequiresApproval
	case cmdrisk.ActionReject:
		action = ActionReject
	case cmdrisk.ActionError:
		action = ActionReject
	}

	dims := make([]Dimension, 0, len(ca.RiskDimensions))
	for _, d := range ca.RiskDimensions {
		dims = append(dims, Dimension{
			Name:     d.Name,
			Severity: d.Severity,
			Reason:   d.Reason,
		})
	}

	return Assessment{
		Decision:    Decision{Action: action, Reasons: ca.Decision.Reasons, Rationale: ca.Decision.Rationale},
		Dimensions:  dims,
		Confidence:  string(ca.Confidence),
		Explanation: ca.Explanation.Summary,
	}
}



// Compile-time checks.
var (
	_ IntentAssessor = (*CmdRiskAssessor)(nil)
	_ IntentAcceptor = (*CmdRiskAssessor)(nil)
)
