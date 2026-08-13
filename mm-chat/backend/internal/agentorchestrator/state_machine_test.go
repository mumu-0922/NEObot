package agentorchestrator

import "testing"

func TestStateMachinesMatchPhaseZeroContract(t *testing.T) {
	tests := []struct {
		name  string
		valid func(string, string) bool
		good  [][2]string
		bad   [][2]string
	}{
		{"run", ValidRunTransition,
			[][2]string{{RunPending, RunAdmitted}, {RunAdmitted, RunQueued}, {RunQueued, RunRunning}, {RunRunning, RunOutcomeUnknown}},
			[][2]string{{RunPending, RunRunning}, {RunQueued, RunSucceeded}, {RunSucceeded, RunKilled}}},
		{"step", ValidStepTransition,
			[][2]string{{StepPending, StepReady}, {StepReady, StepRunning}, {StepRunning, StepSucceeded}},
			[][2]string{{StepPending, StepRunning}, {StepReady, StepSucceeded}, {StepSucceeded, StepFailed}}},
		{"attempt", ValidAttemptTransition,
			[][2]string{{AttemptLeased, AttemptStarting}, {AttemptStarting, AttemptRunning}, {AttemptRunning, AttemptPrepared}, {AttemptPrepared, AttemptCommitting}, {AttemptCommitting, AttemptOutcomeUnknown}},
			[][2]string{{AttemptLeased, AttemptRunning}, {AttemptPrepared, AttemptSucceeded}, {AttemptLeaseExpired, AttemptLeased}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, edge := range test.good {
				if !test.valid(edge[0], edge[1]) {
					t.Fatalf("legal edge %q -> %q rejected", edge[0], edge[1])
				}
			}
			for _, edge := range test.bad {
				if test.valid(edge[0], edge[1]) {
					t.Fatalf("illegal edge %q -> %q accepted", edge[0], edge[1])
				}
			}
		})
	}
}

func TestTerminalPrecedenceIsConservativeOrderingOnly(t *testing.T) {
	ordered := []string{RunSucceeded, RunFailed, RunCanceled, RunKilled, RunOutcomeUnknown}
	for index, state := range ordered {
		if TerminalRank(state) != index+1 || !IsRunTerminal(state) {
			t.Fatalf("terminal rank %q = %d", state, TerminalRank(state))
		}
	}
	if TerminalRank(RunRunning) != 0 || IsRunTerminal(RunRunning) {
		t.Fatal("nonterminal Run received terminal authority")
	}
}
