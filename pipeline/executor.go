package pipeline

import (
	"context"
	"time"
)

// Executor 执行者
type Executor interface {
	//Prepare 准备
	Prepare(ctx context.Context) error
	//Do 执行
	Do(ctx context.Context, param ExecutorParam) <-chan ExecutorResult
	//Destruction 销毁环境
	Destruction(ctx context.Context) error
}

// ExecutorParam 执行参数
type ExecutorParam interface {
	//GetCmd 获取执行的指令
	GetCmd() string
	//GetTimeout 获取执行超时时间
	GetTimeout() time.Duration
	GetRuntimeInfo() interface{}
	GetStep() Step
}

// ExecutorResult 执行返回
type ExecutorResult interface {
	GetLog() ExecutorLog
	GetErr() error
}

type ExecutorLog struct {
	Error   string
	Success string
}
