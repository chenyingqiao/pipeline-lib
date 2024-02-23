package steps

import (
	"context"
	"strings"
	"text/template"
	"time"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/templete"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
)

type Kv struct {
	pipeline.Metadata `mapstructure:",squash"`
}

// File 文件
type File struct {
	Path    string `mapstructure:"path"`
	Content string `mapstructure:"content"`
}

func KvToK8sEnvVars(kv []Kv) []v1.EnvVar {
	envs := []v1.EnvVar{}
	for _, item := range kv {
		envs = append(envs, v1.EnvVar{
			Name:  item.Key,
			Value: item.Value,
		})
	}
	return envs
}

// MapToKv
func MapToKv(m map[string]string) []Kv {
	result := []Kv{}
	for k, v := range m {
		result = append(result, Kv{
			Metadata: pipeline.Metadata{
				Key:   k,
				Value: v,
			},
		})
	}
	return result
}

// KvToMap
func KvToMap(kv []Kv) map[string]string {
	result := map[string]string{}
	for _, v := range kv {
		result[v.Key] = v.Value
	}
	return result
}

// BaseStep 基础步骤
type BaseStep struct {
	id         string
	stageID    string
	pipelineID string
	status     string
	log        string
	logID      string
	code       string
	timeout    time.Duration
	stage      pipeline.StageOperator
	pipeline   pipeline.PipelineOperator
	err        error
}

// NewBaseStep 实例化基础节点
func NewBaseStep(p pipeline.PipelineOperator, stage pipeline.StageOperator, id string) BaseStep {
	if id == "" {
		id = uuid.New().String()
	}
	b := BaseStep{
		id:         id,
		stageID:    stage.GetID(),
		pipelineID: p.GetID(),
		status:     pipeline.StepStatusUnknown,
		log:        "",
		stage:      stage,
		pipeline:   p,
	}
	return b
}

// GetID 获取节点id
func (b *BaseStep) GetID() string {
	return b.id
}

// GetStageID 获取步骤id
func (b *BaseStep) GetStageID() string {
	return b.stageID
}

// GetPipelineID 获取流水线id
func (b *BaseStep) GetPipelineID() string {
	return b.pipelineID
}

// GetStatus 获取状态
func (b *BaseStep) GetStatus() string {
	return b.status
}

// GetCode 获取节点标签
func (b *BaseStep) GetCode() string {
	return b.status
}

// SetStatus 获取状态
func (b *BaseStep) SetStatus(status string) {
	util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), b.pipeline), "流水线: %s stage: %s step: %s 状态变化：%s -> %s", b.pipeline.GetID(), b.stage.GetID(), b.id, b.status, status)
	b.status = status
	// util.UpdateLog(pipeline.ContextWithPipelineImpl(context.Background(), b.pipeline), b.pipeline, b.GetID(), status, util.UpdateState)
}

// GetLog 获取日志信息
func (b *BaseStep) GetLog() string {
	return b.log
}

func (b *BaseStep) AdditionLog(log string) {
	b.log += log
	b.pipeline.Notify()
}

// GetErr 获取错误信息
func (b *BaseStep) GetErr() error {
	return b.err
}

// Exec 执行
func (b *BaseStep) Exec(ctx context.Context) error {
	b.logID = uuid.New().String()
	return nil
}

func (b *BaseStep) initPublicVars() {
	//step公共变量设置
	b.pipeline.SetMetadata("PipelineStageID", b.stageID)
	b.pipeline.SetMetadata("PipelineStepID", b.id)
	b.pipeline.SetMetadata("PipelineConfigPath", util.GetStepConfigPath(b))
	//修改PipelineWorkDir位置信息
	b.pipeline.SetMetadata("PIPELINX_WORKDIR", util.GetStepWorkdir(b))
	b.pipeline.SetMetadata("PipelineWorkdir", util.GetStepWorkdir(b))
}

// GetLogID 获取logID
func (b *BaseStep) GetLogID() string {
	return b.logID
}

func (b *BaseStep) isTermination(termination pipeline.Termination) bool {
	if termination.When == "" {
		termination.When = "{{isFail}}"
	}
	result := b.ParseSimple(context.Background(), termination.When, map[string]interface{}{}, template.FuncMap{
		"isSuccess": func() bool {
			return b.err == nil
		},
		"isFail": func() bool {
			return b.err != nil
		},
		"isIgnore": func() bool {
			//支持节点
			return false
		},
	})
	if strings.Trim(result, " ") == "true" {
		return true
	}
	return false
}

// ParseSimple 简单解析
func (b *BaseStep) ParseSimple(ctx context.Context, temp string, data map[string]interface{}, fnMap template.FuncMap) string {
	data = util.Merge(data, b.pipeline.GetMetadata())
	return templete.Parse(
		ctx,
		temp,
		data,
		false,
		fnMap,
	)
}

// ParseObject 泛型转换Object
func ParseObject[T any](ctx context.Context, p pipeline.PipelineOperator, stage pipeline.Stage, step pipeline.Step, temp T) T {
	data := p.GetMetadata()
	return templete.ParseObject(
		ctx,
		temp,
		data,
		templete.GetDefaultParseFnMap(
			ctx,
			p,
			stage,
			step,
			nil,
			data,
		),
	)
}

// ChainEach 获取step实例
func ChainEach(p pipeline.PipelineOperator, stage pipeline.StageOperator, data interface{}) (pipeline.StepOperator, error) {
	chain := []pipeline.NewStepFn{}
	chain = append(chain, []pipeline.NewStepFn{
		NewK8sStep,
		NewLocalStep,
	}...)
	//模板替换
	for _, item := range chain {
		re, err := item(p, stage, data)
		if errors.Is(err, pipeline.ErrStepTypeNotMatch) {
			continue
		}
		if err == nil {
			re.GetStatus()
			return re, nil
		}
		return nil, err
	}
	return nil, errors.New("step not match!")
}

func (j *BaseStep) GetLogFromExecutorResult(result pipeline.ExecutorResult) (string, error) {
	defer j.pipeline.Notify()
	log := result.GetLog().Success + result.GetLog().Error
	if result.GetErr() != nil {
		j.err = result.GetErr()
		return log + result.GetErr().Error(), result.GetErr()
	}
	return log, nil
}

func GetTimeout(p pipeline.Pipeline) int64 {
	return p.GetConfig().Config.Timeout
}
