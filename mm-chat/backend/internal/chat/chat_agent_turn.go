package chat

const (
	maxChatAgentTurnSteps = 32
	maxChatAgentToolCalls = 128
)

type chatAgentStepPurpose string

const (
	chatAgentStepSkillPrelude chatAgentStepPurpose = "skill_prelude"
	chatAgentStepTask         chatAgentStepPurpose = "task"
)

type chatAgentStep struct {
	Sequence     int
	TaskSequence int
	Purpose      chatAgentStepPurpose
}

type chatAgentTurnDriver struct {
	steps            int
	taskSteps        int
	toolCalls        int
	completionDriven bool
}

func newChatAgentTurnDriver(completionDriven ...bool) *chatAgentTurnDriver {
	driver := &chatAgentTurnDriver{}
	if len(completionDriven) > 0 {
		driver.completionDriven = completionDriven[0]
	}
	return driver
}

func (driver *chatAgentTurnDriver) beginStep(skillPrelude bool) (chatAgentStep, bool) {
	if driver == nil || (!driver.completionDriven && driver.steps >= maxChatAgentTurnSteps) {
		return chatAgentStep{}, false
	}
	driver.steps++
	step := chatAgentStep{Sequence: driver.steps, Purpose: chatAgentStepTask}
	if skillPrelude {
		step.Purpose = chatAgentStepSkillPrelude
		step.TaskSequence = driver.taskSteps
		return step, true
	}
	driver.taskSteps++
	step.TaskSequence = driver.taskSteps
	return step, true
}

func (driver *chatAgentTurnDriver) admitToolCalls(requested int) int {
	if driver == nil || requested <= 0 ||
		(!driver.completionDriven && driver.toolCalls >= maxChatAgentToolCalls) {
		return 0
	}
	if driver.completionDriven {
		driver.toolCalls += requested
		return requested
	}
	admitted := min(requested, maxChatAgentToolCalls-driver.toolCalls)
	driver.toolCalls += admitted
	return admitted
}
