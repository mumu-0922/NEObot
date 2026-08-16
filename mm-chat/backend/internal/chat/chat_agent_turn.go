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
	steps     int
	taskSteps int
	toolCalls int
}

func newChatAgentTurnDriver() *chatAgentTurnDriver {
	return &chatAgentTurnDriver{}
}

func (driver *chatAgentTurnDriver) beginStep(skillPrelude bool) (chatAgentStep, bool) {
	if driver == nil || driver.steps >= maxChatAgentTurnSteps {
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
	if driver == nil || requested <= 0 || driver.toolCalls >= maxChatAgentToolCalls {
		return 0
	}
	admitted := min(requested, maxChatAgentToolCalls-driver.toolCalls)
	driver.toolCalls += admitted
	return admitted
}
