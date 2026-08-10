package configure

import (
	"context"
	"fmt"
)

// ApplyFailure records one configuration change that could not be applied.
type ApplyFailure struct {
	Change Change
	Err    error
}

// ApplyResult reports every successful and failed configuration change.
type ApplyResult struct {
	Applied  []Change
	Failures []ApplyFailure
}

// Apply performs every change in a previously confirmed plan and reports all outcomes.
func (m *Manager) Apply(ctx context.Context, changes []Change) ApplyResult {
	result := ApplyResult{
		Applied:  make([]Change, 0, len(changes)),
		Failures: make([]ApplyFailure, 0),
	}
	for _, change := range changes {
		var err error
		switch change.Component {
		case "MCP":
			if configureErr := m.configureMCP(ctx, change.Agent); configureErr != nil {
				err = fmt.Errorf("configure %s MCP: %w", change.AgentName, configureErr)
			}
		case "skill":
			if installErr := m.installSkill(change.Target); installErr != nil {
				err = fmt.Errorf("install %s skill: %w", change.AgentName, installErr)
			}
		default:
			err = fmt.Errorf("unknown configuration component %q", change.Component)
		}
		if err != nil {
			result.Failures = append(result.Failures, ApplyFailure{Change: change, Err: err})
			continue
		}
		result.Applied = append(result.Applied, change)
	}
	return result
}
