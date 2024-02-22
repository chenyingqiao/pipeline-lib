package executor

import (
	"context"
	"time"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
)

//ExecutorParam k8s执行参数
type ExecutorParam struct {
	CMD         string
	Timeout     time.Duration
	RuntimeInfo interface{}
	Step        pipeline.Step
}

//K8sRuntimeInfo 运行时信息
type K8sRuntimeInfo struct {
	Namespace         string
	PodName           string
	InitContainerName []string
	ContainerName     string
	Files             []K8sFiles
	SuccessWriter     *util.BufferWriterSync
	ErrorWriter       *util.BufferWriterSync
	Context           context.Context
	Cancel            context.CancelFunc
}

//SetInitContainerName 设置初始化容器名
func (k *K8sRuntimeInfo) SetInitContainerName(initContainerName ...string) {
	k.InitContainerName = initContainerName
}

//GetInitContainerName 获取初始化容器名称
func (k *K8sRuntimeInfo) GetInitContainerName() []string {
	return k.InitContainerName
}

//NewK8sRuntimeInfo 实例化k8s运行时
func NewK8sRuntimeInfo(p pipeline.PipelineOperator, step pipeline.StepOperator, files []K8sFiles) K8sRuntimeInfo {
	runtimeInfoCtx, runtimeInfoCancel := context.WithCancel(context.Background())
	return K8sRuntimeInfo{
		Namespace:     "default",
		PodName:       NewK8sExecutor(p).getPodName(),
		ContainerName: util.GetStepContainerName(step),
		Files:         files,
		SuccessWriter: util.NewBufferWriterSync(),
		ErrorWriter:   util.NewBufferWriterSync(),
		Context:       runtimeInfoCtx,
		Cancel:        runtimeInfoCancel,
	}
}

//K8sFiles 文件列表
type K8sFiles struct {
	Path    string `mapstructure:"path"`
	Content string `mapstructure:"content"`
}

//GetCmd 获取执行的指令
func (kep ExecutorParam) GetCmd() string {
	return kep.CMD
}

//GetTimeout 获取执行超时时间
func (kep ExecutorParam) GetTimeout() time.Duration {
	return kep.Timeout
}

//GetRuntimeInfo 运行时
func (kep ExecutorParam) GetRuntimeInfo() interface{} {
	return kep.RuntimeInfo
}

//GetStep 获取当前执行的step
func (kep ExecutorParam) GetStep() pipeline.Step {
	return kep.Step
}
