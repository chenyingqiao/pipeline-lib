package stages

import (
	"context"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/steps"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
	"golang.org/x/sync/errgroup"
)

// Stage k8s自定义执行stage
type Stage struct {
	id          string
	log         string
	name        string
	code        string
	stype       string
	status      string
	parallelism bool
	err         error
	steps       []pipeline.StepOperator
	stages      []pipeline.StageOperator
	config      map[string]interface{}
	p           pipeline.PipelineOperator
	retry       Retry
	isStarted   bool
	termination bool
}

// NewStage 实例化stage
func NewStage(p pipeline.PipelineOperator, data interface{}) (pipeline.StageOperator, error) {
	//参数校验
	if _, ok := data.(map[string]interface{}); !ok {
		return nil, errors.New("data type error: need map[string]string")
	}
	dataArr := data.(map[string]interface{})
	err := validation(dataArr)
	if err != nil {
		return nil, err
	}

	stage := Stage{}
	stage.id = cast.ToString(dataArr["id"])
	if stage.id == "" {
		stage.id = uuid.New().String()
	}
	stage.name = cast.ToString(dataArr["name"])
	stage.parallelism = cast.ToBool(dataArr["parallelism"])
	stage.termination = cast.ToBool(dataArr["termination"])
	stage.steps = []pipeline.StepOperator{}
	stage.stype = cast.ToString(dataArr["type"])
	stage.code = cast.ToString(dataArr["code"])
	stage.stages = []pipeline.StageOperator{}
	stage.config = dataArr
	stage.p = p
	stage.retry.R = cast.ToBool(dataArr["retry"])
	stage.retry.Ask = make(chan interface{})
	stage.status = pipeline.StageStatusUnknown
	err = getStageList(p, &stage, dataArr)
	if err != nil {
		return nil, err
	}
	err = getStepList(p, &stage, dataArr)
	if err != nil {
		return nil, err
	}

	if len(stage.GetStages()) != 0 && len(stage.GetSteps()) != 0 {
		return nil, errors.New("多个step和多个stage不能同时存在")
	}
	if stage.stype != "" {
		return nil, pipeline.ErrStageTypeNotMatch
	}
	return &stage, nil
}

func validation(data map[string]interface{}) error {
	if !util.IsExist(data, []string{"name"}...) {
		return errors.New("missing param: name")
	}
	if !util.IsExist(data, []string{"steps"}...) && !util.IsExist(data, []string{"stages"}...) {
		return errors.New("missing param: steps or stages")
	}
	return nil
}

// getSteps 初始化step数据
func getStepList(p pipeline.PipelineOperator, stage pipeline.StageOperator, data map[string]interface{}) error {
	stepsIface := util.GetV(data, "steps", interface{}([]interface{}{}))
	if stepsIface == nil {
		stepsIface = []interface{}{}
	}
	if _, ok := stepsIface.([]interface{}); !ok {
		return pipeline.ErrCast
	}
	for _, item := range stepsIface.([]interface{}) {
		step, err := steps.ChainEach(p, stage, item)
		if err != nil {
			return err
		}
		if step == nil {
			continue
		}
		stage.AddStep(step)
	}
	return nil
}

func getStageList(p pipeline.PipelineOperator, stage pipeline.StageOperator, data map[string]interface{}) error {
	stagesIface := util.GetV(data, "stages", interface{}([]interface{}{}))
	if stagesIface == nil {
		stagesIface = []interface{}{}
	}
	if _, ok := stagesIface.([]interface{}); !ok {
		return pipeline.ErrCast
	}
	stages, err := util.ParseConfigToStages(p, stagesIface.([]interface{}), p.GetStagesFn()...)
	if err != nil {
		return err
	}
	for _, item := range stages {
		stage.AddStage(item)
	}
	return nil
}

// AddStage 添加stage
func (kcs *Stage) AddStage(stage pipeline.StageOperator) error {
	kcs.stages = append(kcs.stages, stage)
	return nil
}

// GetStages 获取stage列表
func (kcs *Stage) GetStages() []pipeline.StageOperator {
	return kcs.stages
}

// GetID 获取stage id标识
func (kcs *Stage) GetID() string {
	return kcs.id
}

// GetStatus 获取状态
func (kcs *Stage) GetStatus() string {
	return kcs.status
}

// SetStatus 获取状态
func (kcs *Stage) SetStatus(status string) {
	defer kcs.p.Notify()
	util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), kcs.p), "流水线%s stage:%s 状态变化：%s -> %s", kcs.p.GetID(), kcs.id, kcs.status, status)
	kcs.status = status
	// util.UpdateLog(pipeline.ContextWithPipelineImpl(context.Background(), kcs.p), kcs.p, kcs.id, kcs.status, util.UpdateState)
}

// GetLog 获取日志
func (kcs *Stage) GetLog() string {
	return kcs.log
}

// GetSteps 获取步骤
func (kcs *Stage) GetSteps() []pipeline.StepOperator {
	return kcs.steps
}

// GetName 获取名称
func (kcs *Stage) GetName() string {
	return kcs.name
}

// Answer 应答
func (kcs *Stage) Answer(ctx context.Context, data interface{}) error {
	kcs.retry.answer(ctx, data)
	return nil
}

// Start 执行step
func (kcs *Stage) Start(ctx context.Context) error {
	defer func() {
		kcs.isStarted = false
	}()
	if len(kcs.GetStages()) != 0 {
		if kcs.parallelism {
			//并行
			ewg := errgroup.Group{}
			for _, stage := range kcs.GetStages() {
				kcs.SetStatus(pipeline.StageStatusRunning)
				stageItem := stage
				ewg.Go(func() error {
					err := stageItem.Start(ctx)
					if err != nil {
						stageItem.SetStatus(pipeline.StageStatusFailed)
						if !kcs.termination && !errors.Is(err, pipeline.ErrTimeoutOrCancel) {
							return nil
						}
						return err
					}
					stageItem.SetStatus(pipeline.StageStatusSuccess)
					return nil
				})
			}
			if err := ewg.Wait(); err != nil {
				kcs.SetStatus(pipeline.StepStatusFailed)
				kcs.err = err
				if !kcs.termination && !errors.Is(err, pipeline.ErrTimeoutOrCancel) {
					return nil
				}
				return err
			}
			kcs.SetStatus(pipeline.StageStatusSuccess)
		} else {
			//串行
			for _, stage := range kcs.GetStages() {
				kcs.SetStatus(pipeline.StageStatusRunning)
				if err := stage.Start(ctx); err != nil {
					kcs.err = err
					kcs.SetStatus(pipeline.StepStatusFailed)
					if !kcs.termination && !errors.Is(err, pipeline.ErrTimeoutOrCancel) {
						return nil
					}
					return err
				}
				kcs.SetStatus(pipeline.StageStatusSuccess)
			}
		}
	}
	if len(kcs.GetSteps()) != 0 {
		err := kcs.stepStart(ctx)
		if err != nil {
			kcs.err = err
			kcs.SetStatus(pipeline.StepStatusFailed)
			if !kcs.termination && !errors.Is(err, pipeline.ErrTimeoutOrCancel) {
				return nil
			}
			return err
		}
		kcs.SetStatus(pipeline.StageStatusSuccess)
	}
	return nil
}

// GetErr 获取运行报错
func (kcs *Stage) GetErr() error {
	return kcs.err
}

func (kcs *Stage) GetCode() string {
	return kcs.code
}

// stepStart 执行step
func (kcs *Stage) stepStart(ctx context.Context) error {
	steps := kcs.steps
	for _, step := range steps {
		step.SetStatus(pipeline.StepStatusRunning)
		//单节点重试
		err := kcs.retry.startRetry(ctx, kcs.p, kcs, step.Exec)
		if !step.IsTermination() {
			step.SetStatus(pipeline.StepStatusSuccess)
			continue
		}
		if err != nil && util.IsTerminatorErr(err) {
			step.SetStatus(pipeline.StepStatusTerminate)
			return err
		}
		if err != nil {
			step.SetStatus(pipeline.StepStatusFailed)
			return err
		}
		step.SetStatus(pipeline.StepStatusSuccess)
		return err
	}
	return nil
}

// AddStep 添加step
func (kcs *Stage) AddStep(step pipeline.StepOperator) error {
	if kcs.isStarted {
		kcs.SetStatus(pipeline.StageStatusFailed)
		return errors.New("stage is started, cannot add step in running")
	}
	kcs.steps = append(kcs.steps, step)
	return nil
}

// GetConfig 获取配置信息
func (kcs *Stage) GetConfig() map[string]interface{} {
	return kcs.config
}
