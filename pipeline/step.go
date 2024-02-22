package pipeline

import (
	"context"
)

var (
	//K8s k8s
	K8s EnvType = "k8s"
	//Local 本地模式
	Local EnvType = "local"
)

const (
	StepStatusRunning   = "RUNNING"
	StepStatusFailed    = "FAILED"
	StepStatusSuccess   = "SUCCESS"
	StepStatusTerminate = "ABORTED"
	StepStatusPaused    = "PAUSED"
	StepStatusUnknown   = "UNKNOWN"
	StepStatusSkipped   = "SKIPPED"
)

// BaseParam 基础参数
type BaseParam struct {
	ID          string      `mapstructure:"id"`
	Type        string      `mapstructure:"type"`
	Name        string      `mapstructure:"name"`
	Code        string      `mapstructure:"code"`
	Image       string      `mapstructure:"image"`
	Metadata    []Metadata  `mapstructure:"metadata"`
	Termination Termination `mapstructure:"termination"`
}

// Termination 终止条件
type Termination struct {
	When string `mapstructure:"when"`
}

// Metadata 元数据
type Metadata struct {
	Key   string `mapstructure:"key"`
	Value string `mapstructure:"value"`
}

type EnvType string
type NewStepFn func(p PipelineOperator, stage StageOperator, data interface{}) (StepOperator, error)

// Step 最小运行步骤
type Step interface {
	GetID() string
	GetStageID() string
	GetPipelineID() string
	GetStatus() string
	GetLog() string
	AdditionLog(log string)
}

// StepOperator 执行步骤操作者
type StepOperator interface {
	Step
	Exec(ctx context.Context) error
	Env() (interface{}, EnvType)
	//IsTermination 是否应该终止下面的步骤
	IsTermination() bool
	SetStatus(status string)
	GetErr() error
	GetLogID() string
	GetBaseParam() BaseParam
}
