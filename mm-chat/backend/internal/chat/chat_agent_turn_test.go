package chat

import "testing"

func TestChatAgentTurnDriverKeepsSkillPreludeOutsideTaskSequence(t *testing.T) {
	driver := newChatAgentTurnDriver()
	first, ok := driver.beginStep(true)
	if !ok || first.Sequence != 1 || first.TaskSequence != 0 ||
		first.Purpose != chatAgentStepSkillPrelude {
		t.Fatalf("first step=%#v ok=%v", first, ok)
	}
	second, ok := driver.beginStep(false)
	if !ok || second.Sequence != 2 || second.TaskSequence != 1 ||
		second.Purpose != chatAgentStepTask {
		t.Fatalf("second step=%#v ok=%v", second, ok)
	}
	third, ok := driver.beginStep(false)
	if !ok || third.Sequence != 3 || third.TaskSequence != 2 {
		t.Fatalf("third step=%#v ok=%v", third, ok)
	}
}

func TestChatAgentTurnDriverEnforcesGlobalStepAndCallBudgets(t *testing.T) {
	driver := newChatAgentTurnDriver()
	for sequence := 1; sequence <= maxChatAgentTurnSteps; sequence++ {
		step, ok := driver.beginStep(false)
		if !ok || step.Sequence != sequence {
			t.Fatalf("step %d=%#v ok=%v", sequence, step, ok)
		}
	}
	if _, ok := driver.beginStep(false); ok {
		t.Fatal("step budget admitted an extra Step")
	}
	if admitted := driver.admitToolCalls(maxChatAgentToolCalls - 1); admitted != maxChatAgentToolCalls-1 {
		t.Fatalf("first admitted calls=%d", admitted)
	}
	if admitted := driver.admitToolCalls(4); admitted != 1 {
		t.Fatalf("last admitted calls=%d", admitted)
	}
	if admitted := driver.admitToolCalls(1); admitted != 0 {
		t.Fatalf("over-budget admitted calls=%d", admitted)
	}
}
