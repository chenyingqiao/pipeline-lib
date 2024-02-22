package util

import (
	"fmt"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	v1 "k8s.io/api/core/v1"
)

// CastToK8sEnv 转换为k8s环境参数
func CastToK8sEnv(source interface{}) v1.PodSpec {
	if _, ok := source.(v1.PodSpec); !ok {
		return v1.PodSpec{}
	}
	return source.(v1.PodSpec)
}

// CastToK8sEnvWithErr 转换为k8s环境参数
func CastToK8sEnvWithErr(source interface{}) (v1.PodSpec, error) {
	if _, ok := source.(v1.PodSpec); !ok {
		return v1.PodSpec{}, pipeline.ErrCast
	}
	return source.(v1.PodSpec), nil
}

// SetFirstStageStepInitLog 设置环境初始化日志到第一个节点的日志中
func SetFirstStageStepInitLog(p pipeline.PipelineOperator, log string, args ...interface{}) {
	defer p.Notify()
	log = fmt.Sprintf(log+"\n", args...)
	stepAdditionLog := func(stage pipeline.StageOperator, log string) {
		steps := stage.GetSteps()
		if len(steps) != 0 {
			steps[0].AdditionLog(log)
		}
	}
	stages := p.GetStages()
	if len(stages) != 0 {
		cstages := stages[0].GetStages()
		if len(cstages) != 0 {
			stepAdditionLog(cstages[0], log)
			return
		}
		stepAdditionLog(stages[0], log)
	}
}
