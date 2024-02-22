package steps

import (
	"context"
	"strings"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/executor"
	"github.com/chenyingqiao/pipeline-lib/pipeline/templete"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
)

// LocalCustomStageType yaml中定义的类型
const LocalCustomStageType = "local"

// LocalStep step
type LocalStep struct {
	BaseStep

	cmds     []string
	executor pipeline.Executor
	config   LocalStepParam
	err      error
}

// LocalStepParam 入参格式
type LocalStepParam struct {
	pipeline.BaseParam `mapstructure:",squash"`
	Shell              []string              `mapstructure:"shell"`
	Files              []executor.LocalFiles `mapstructure:"files"`
}

// NewLocalStep 实例化shell
func NewLocalStep(p pipeline.PipelineOperator, stage pipeline.StageOperator, data interface{}) (pipeline.StepOperator, error) {
	if _, ok := data.(map[string]interface{}); !ok {
		return nil, errors.New("data type error: need map[string]string")
	}
	dataMap := data.(map[string]interface{})
	if !util.IsExist(dataMap, "type") || util.GetV(dataMap, "type", "").(string) != LocalCustomStageType {
		return nil, pipeline.ErrStepTypeNotMatch
	}
	if err := util.Check(dataMap, []string{"name", "shell", "type"}); err != nil {
		return nil, err
	}

	dataStruct := &LocalStepParam{}
	err := mapstructure.Decode(dataMap, dataStruct)
	if err != nil {
		return nil, err
	}

	//附加code到metadata中，用于让用户可以通过占位符直接获取数据数据
	dataStruct.Code = cast.ToString(stage.GetConfig()["code"])
	if dataStruct.Code != "" {
		dataStruct.Metadata = append(dataStruct.Metadata, pipeline.Metadata{
			Key:   dataStruct.Code,
			Value: "{{ getOutput }}",
		})
	}

	//生成steps
	step := LocalStep{
		BaseStep: NewBaseStep(p, stage, dataStruct.ID),
		cmds:     dataStruct.Shell,
		config:   *dataStruct,
		executor: executor.NewLocalExecutor(p),
	}
	return &step, nil
}

// Env 环境配置信息
func (kss *LocalStep) Env() (interface{}, pipeline.EnvType) {
	return nil, pipeline.Local
}

// IsTermination 是否终止后续操作
func (kss *LocalStep) IsTermination() bool {
	return kss.isTermination(kss.config.Termination)
}

// Exec 执行当前step
func (kss *LocalStep) Exec(ctx context.Context) error {
	if err := kss.BaseStep.Exec(ctx); err != nil {
		return err
	}
	cmd := strings.Join(kss.cmds, ";\n")
	err := kss.exec(ctx, cmd)
	if err != nil {
		kss.err = err
		return err
	}
	return nil
}

func (kss *LocalStep) exec(ctx context.Context, cmd string) error {
	//解析转换参数中的模板
	kss.config = ParseObject(ctx, kss.pipeline, kss.stage, kss, kss.config)
	//stage配置信息
	runtimeInfo := executor.LocalRuntimeInfo{
		Files: kss.config.Files,
	}
	rchan := kss.executor.Do(ctx, executor.ExecutorParam{
		CMD:         cmd,
		Timeout:     kss.timeout,
		Step:        kss,
		RuntimeInfo: runtimeInfo,
	})
	defer func() {
		// 设置metadata
		for _, item := range kss.config.Metadata {
			kss.pipeline.SetMetadata(
				item.Key,
				templete.Parse(
					ctx,
					item.Value,
					map[string]interface{}{},
					false,
					templete.GetDefaultParseFnMap(ctx, kss.pipeline, kss.stage, kss, runtimeInfo, map[string]interface{}{}),
				),
			)
		}
	}()
	for {
		select {
		case result, ok := <-rchan:
			if !ok {
				return nil
			}
			beforeLog, err := kss.GetLogFronExecutorResult(result)
			kss.LogAppend(beforeLog)
			if util.IsTerminatorErr(err) {
				return pipeline.ErrTimeoutOrCancel
			}
			if err != nil {
				return err
			}
		}
	}
}
func (kss *LocalStep) GetBaseParam() pipeline.BaseParam {
	return kss.config.BaseParam
}
