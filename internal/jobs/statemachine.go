package jobs

// targetTransitions defines the legal per-target state machine.
var targetTransitions = map[string]map[string]bool{
	StatusPending: {
		StatusRunning: true, StatusCancelled: true, StatusSkipped: true, StatusLost: true,
	},
	StatusRunning: {
		StatusSuccess: true, StatusFailed: true, StatusTimeout: true,
		StatusCancelled: true, StatusLost: true,
	},
	StatusSuccess:   {},
	StatusFailed:    {},
	StatusTimeout:   {},
	StatusCancelled: {},
	StatusLost:      {},
	StatusSkipped:   {},
}

// terminalStates cannot leave their state.
var terminalStates = map[string]bool{
	StatusSuccess: true, StatusFailed: true, StatusTimeout: true,
	StatusCancelled: true, StatusLost: true, StatusSkipped: true,
}

// CanTransitionTarget reports whether from->to is legal.
func CanTransitionTarget(from, to string) bool {
	return targetTransitions[from][to]
}

// IsTerminal reports whether a target/run state is final.
func IsTerminal(s string) bool { return terminalStates[s] }

// Aggregate computes the run-level status from target outcomes with a fixed
// priority: cancelled (whole run aborted) > timeout > failed > lost > success.
func Aggregate(statuses []string) string {
	if len(statuses) == 0 {
		return StatusSuccess
	}
	counts := RunSummary{Total: len(statuses)}
	allSuccess := true
	for _, s := range statuses {
		switch s {
		case StatusPending:
			counts.Pending++
		case StatusRunning:
			counts.Running++
		case StatusSuccess:
			counts.Success++
		case StatusFailed:
			counts.Failed++
		case StatusTimeout:
			counts.Timeout++
		case StatusCancelled:
			counts.Cancelled++
		case StatusLost:
			counts.Lost++
		case StatusSkipped:
			counts.Skipped++
		}
		if s != StatusSuccess && s != StatusSkipped {
			allSuccess = false
		}
	}
	switch {
	case counts.Pending > 0 || counts.Running > 0:
		return StatusRunning
	case allSuccess:
		return StatusSuccess
	case counts.Cancelled > 0 && counts.Failed == 0 && counts.Timeout == 0 && counts.Lost == 0:
		return StatusCancelled
	case counts.Timeout > 0:
		return StatusTimeout
	case counts.Failed > 0:
		return StatusFailed
	case counts.Lost > 0:
		return StatusLost
	case counts.Cancelled > 0:
		return StatusCancelled
	default:
		return StatusSuccess
	}
}

// Summarise returns counts for a set of target states.
func Summarise(statuses []string) RunSummary {
	s := RunSummary{Total: len(statuses)}
	for _, st := range statuses {
		switch st {
		case StatusPending:
			s.Pending++
		case StatusRunning:
			s.Running++
		case StatusSuccess:
			s.Success++
		case StatusFailed:
			s.Failed++
		case StatusTimeout:
			s.Timeout++
		case StatusCancelled:
			s.Cancelled++
		case StatusLost:
			s.Lost++
		case StatusSkipped:
			s.Skipped++
		}
	}
	return s
}
