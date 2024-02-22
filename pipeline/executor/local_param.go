package executor

import (
	"time"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
)

//LocalExecutorParam 本地执行参数
type LocalExecutorParam struct {
	ExecutorParam
}

//GetCmd 获取执行的指令
func (l *LocalExecutorParam) GetCmd() string {
	return ""
}

//GetTimeout 获取执行超时时间
func (l *LocalExecutorParam) GetTimeout() time.Duration {
	return time.Second
}

//GetRuntimeInfo 运行时
func (l *LocalExecutorParam) GetRuntimeInfo() interface{} {
	return nil
}

//GetStep 获取当前执行的step
func (l *LocalExecutorParam) GetStep() pipeline.Step {
	return nil
}

//LocalRuntimeInfo 本地运行环境的runtimeinfo
type LocalRuntimeInfo struct {
	Files []LocalFiles
}

//LocalFiles 本地文件
type LocalFiles struct {
	Path    string `mapstructure:"path"`
	Content string `mapstructure:"content"`
}
