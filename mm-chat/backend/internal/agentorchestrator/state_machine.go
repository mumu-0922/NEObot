package agentorchestrator

var runTransitions = map[string]map[string]struct{}{
	RunPending:  set(RunAdmitted, RunCanceled, RunKilled, RunFailed),
	RunAdmitted: set(RunQueued, RunCanceled, RunKilled, RunFailed),
	RunQueued:   set(RunRunning, RunCanceled, RunKilled, RunFailed),
	RunRunning:  set(RunSucceeded, RunFailed, RunCanceled, RunKilled, RunOutcomeUnknown),
}

var stepTransitions = map[string]map[string]struct{}{
	StepPending: set(StepReady, StepSkipped, StepCanceled, StepKilled, StepFailed),
	StepReady:   set(StepRunning, StepSkipped, StepCanceled, StepKilled, StepFailed),
	StepRunning: set(StepSucceeded, StepFailed, StepCanceled, StepKilled, StepOutcomeUnknown),
}

var attemptTransitions = map[string]map[string]struct{}{
	AttemptLeased:     set(AttemptStarting, AttemptFailed, AttemptCanceled, AttemptKilled, AttemptLeaseExpired),
	AttemptStarting:   set(AttemptRunning, AttemptFailed, AttemptCanceled, AttemptKilled, AttemptLeaseExpired),
	AttemptRunning:    set(AttemptPrepared, AttemptFailed, AttemptCanceled, AttemptKilled, AttemptLeaseExpired),
	AttemptPrepared:   set(AttemptCommitting, AttemptFailed, AttemptCanceled, AttemptKilled, AttemptLeaseExpired),
	AttemptCommitting: set(AttemptSucceeded, AttemptFailed, AttemptKilled, AttemptOutcomeUnknown),
}

var terminalPrecedence = map[string]int{
	RunSucceeded: 1, RunFailed: 2, RunCanceled: 3, RunKilled: 4, RunOutcomeUnknown: 5,
}

func set(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func validTransition(machine map[string]map[string]struct{}, from, to string) bool {
	_, ok := machine[from][to]
	return ok
}

func ValidRunTransition(from, to string) bool {
	return validTransition(runTransitions, from, to)
}

func ValidStepTransition(from, to string) bool {
	return validTransition(stepTransitions, from, to)
}

func ValidAttemptTransition(from, to string) bool {
	return validTransition(attemptTransitions, from, to)
}

func IsRunTerminal(state string) bool {
	return terminalPrecedence[state] > 0
}

func IsStepTerminal(state string) bool {
	return state == StepSucceeded || state == StepFailed || state == StepSkipped ||
		state == StepCanceled || state == StepKilled || state == StepOutcomeUnknown
}

func IsAttemptTerminal(state string) bool {
	return state == AttemptSucceeded || state == AttemptFailed || state == AttemptCanceled ||
		state == AttemptKilled || state == AttemptOutcomeUnknown || state == AttemptLeaseExpired
}

// TerminalRank is only for reconciling conflicting observations. It must not
// be used to overwrite the first durable terminal transition.
func TerminalRank(state string) int { return terminalPrecedence[state] }
