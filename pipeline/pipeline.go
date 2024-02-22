package pipeline

import (
	"context"

	"github.com/chenyingqiao/pipeline-lib/pipeline/config"
)

const (
	RuntimeK8s = "k8s"

	StatusRunning       = "RUNNING"
	StatusFailed        = "FAILED"
	StatusSuccess       = "SUCCESS"
	StatusTerminate     = "ABORTED"
	StatusPaused        = "PAUSED"
	PipelineStatusKey   = "PIPELINE-STATUS"
	PipelineMetadataKey = "PIPELINE-METADATA"

	PipelineConfigFilename = ""
)

// PipelineListeningFn 流水线监听函数
type PipelineListeningFn func(p Pipeline)

// Pipeline 流水线接口
type Pipeline interface {
	GetID() string
	GetName() string
	// GetLog() Log
	GetStatus() string
	GetRuntime() Runtime
	GetConfig() config.Pipeline
	GetMetadata() map[string]interface{}
	GetErr() error
	Listening(fn PipelineListeningFn) error
	Done() <-chan struct{}
	GetParentID() string
}

// PipelineOperator 流水线操作者
type PipelineOperator interface {
	Pipeline
	AddStage(stage StageOperator)
	GetStages() []StageOperator
	SetStatus(status string)
	Notify()
	Run(ctx context.Context) error
	Cancel()
	SetMetadata(key string, value interface{})
	GetStagesFn() []NewStage
	RegisterImpl(ctx context.Context, impl ...interface{})
}
