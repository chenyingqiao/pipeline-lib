package pipeline

import "context"

const (
	StageStatusRunning   = "RUNNING"
	StageStatusFailed    = "FAILED"
	StageStatusSuccess   = "SUCCESS"
	StageStatusTerminate = "ABORTED"
	StageStatusPaused    = "PAUSED"
	StageStatusUnknown   = "UNKNOWN"
)

// Stage stage对象
type Stage interface {
	GetID() string
	GetLog() string
	GetName() string
	GetStatus() string
	GetCode() string
}

// StageOperator 内部操作类
type StageOperator interface {
	Stage
	AddStep(step StepOperator) error
	Start(ctx context.Context) error
	GetSteps() []StepOperator
	AddStage(stage StageOperator) error
	GetStages() []StageOperator
	GetConfig() map[string]interface{}
	SetStatus(status string)
	GetErr() error
	Answer(ctx context.Context, data interface{}) error
}

type StageCallback interface {
	OnInit(ctx context.Context, p Pipeline, s Stage)
	OnStart(ctx context.Context, p Pipeline, s Stage)
	OnRunning(ctx context.Context, p Pipeline, s Stage)
	OnFinish(ctx context.Context, p Pipeline, s Stage)
}

// NewStage 实例化stage
type NewStage func(p PipelineOperator, data interface{}) (StageOperator, error)
