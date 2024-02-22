package pipe

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/config"
	"github.com/chenyingqiao/pipeline-lib/pipeline/executor"
	"github.com/chenyingqiao/pipeline-lib/pipeline/templete"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
)

// PipelineImpl 流水线实现类
type PipelineImpl struct {
	id               string
	pid              string
	nextID           uint
	log              pipeline.Log
	name             string
	status           string
	runtime          pipeline.Runtime
	metadata         sync.Map
	listening        chan struct{}
	listeningCloseed bool
	listeningLock    sync.Mutex
	done             chan struct{}
	listeningFn      pipeline.PipelineListeningFn
	stages           []pipeline.StageOperator
	lock             sync.Mutex
	config           config.Pipeline
	onceDone         sync.Once
	isCancel         bool
	err              error
	params           config.Params
	stagesFn         []pipeline.NewStage

	runCtx    context.Context
	runCancel context.CancelFunc
}

// NewPipelineImpl 流水线实现类
func NewPipelineImpl(ctx context.Context, runtime pipeline.Runtime, id string, configYaml string, fn ...pipeline.NewStage) (*PipelineImpl, error) {
	//获取入参
	pConfig, err := config.NewPipelineConfig(configYaml)
	if err != nil {
		return nil, err
	}
	//实例化流水线
	if pConfig.ID != "" {
		id = pConfig.ID
	}
	if id == "" {
		return nil, errors.New("未指定流水线id")
	}

	p := &PipelineImpl{
		id:               id,
		pid:              pConfig.PID,
		nextID:           0,
		name:             id,
		status:           pipeline.StatusRunning,
		runtime:          runtime,
		metadata:         sync.Map{},
		listening:        make(chan struct{}),
		listeningCloseed: false,
		listeningLock:    sync.Mutex{},
		done:             make(chan struct{}),
		stages:           []pipeline.StageOperator{},
		lock:             sync.Mutex{},
		config:           pConfig,
		onceDone:         sync.Once{},
		stagesFn:         fn,
		params:           pConfig.Params,
	}
	p.getMetadataFromConf(pConfig.Params.Data)
	p.startListening()
	//设置param到Metadata
	for k, v := range p.getParam() {
		p.SetMetadata(k, v)
	}
	p.SetMetadata("PipelineID", id)
	p.SetMetadata("PipelineStarttime", cast.ToString(time.Now().Unix()))
	data := util.Merge(p.params.Data, p.GetMetadata())
	p.config = templete.ParseObject(ctx, p.config, data, templete.GetDefaultParseFnMap(ctx, p, nil, nil, nil, data))
	return p, nil
}

// getMetadataFromConf 支持通过配置文件初始化metadata
func (p *PipelineImpl) getMetadataFromConf(config map[string]interface{}) error {
	for _, v := range p.config.Metadata {
		p.metadata.Store(v.Key, v.Value)
	}
	return nil
}

func (p *PipelineImpl) GetParentID() string {
	return p.pid
}

// GetErr 获取错误信息
func (p *PipelineImpl) GetErr() error {
	return p.err
}

// GetID 获取流水线id
func (p *PipelineImpl) GetID() string {
	return p.id
}

// GetName 获取名称
func (p *PipelineImpl) GetName() string {
	return p.name
}

// GetLog 获取日志
func (p *PipelineImpl) GetLog() pipeline.Log {
	return p.log
}

// GetStatus 获取流水线运行状态
func (p *PipelineImpl) GetStatus() string {
	return p.status
}

// GetRuntime 获取运行时
func (p *PipelineImpl) GetRuntime() pipeline.Runtime {
	return p.runtime
}

// GetMetadata 获取中继数据
func (p *PipelineImpl) GetMetadata() map[string]interface{} {
	result := map[string]interface{}{}
	p.metadata.Range(func(key, value any) bool {
		result[cast.ToString(key)] = value
		return true
	})
	return result
}

// Cancel 取消
func (p *PipelineImpl) Cancel() {
	if p.runCancel == nil {
		return
	}
	p.isCancel = true
	p.runCancel()
	//如果流水线和节点是Running的就设置为Terminal
	p.SetStatus(pipeline.StatusTerminate)
	for _, stages := range p.stages {
		if stages.GetStatus() != pipeline.StageStatusSuccess {
			stages.SetStatus(pipeline.StageStatusTerminate)
		}
		for _, step := range stages.GetSteps() {
			status := step.GetStatus()
			if status != pipeline.StepStatusSuccess {
				step.SetStatus(pipeline.StepStatusTerminate)
			}
		}
	}
}

// 开始监听
func (p *PipelineImpl) startListening() {
	go func() {
		for {
			select {
			case _, ok := <-p.listening:
				if p.listeningFn != nil {
					p.listeningFn(p)
				}
				if !ok {
					return
				}
			}
		}
	}()
}

// Listening 流水线变动监听
func (p *PipelineImpl) Listening(fn pipeline.PipelineListeningFn) error {
	p.listeningFn = fn
	return nil
}

// AddStage 添加stage节点
func (p *PipelineImpl) AddStage(stage pipeline.StageOperator) {
	p.stages = append(p.stages, stage)
}

// GetStages 获取所有的stage节点
func (p *PipelineImpl) GetStages() []pipeline.StageOperator {
	return p.stages
}

// GetConfig 获取配置信息
func (p *PipelineImpl) GetConfig() config.Pipeline {
	return p.config
}

func (p *PipelineImpl) GetStagesFn() []pipeline.NewStage {
	return p.stagesFn
}

// Run 执行流水线
func (p *PipelineImpl) Run(ctx context.Context) error {
	defer p.clean()
	//初始化配置
	var err error
	if p.config.Config.Timeout == 0 {
		p.config.Config.Timeout = 900
	}
	ctx, p.runCancel = context.WithTimeout(ctx, time.Second*time.Duration(p.config.Config.Timeout))
	ctx = pipeline.ContextWithPipelineImpl(ctx, p)
	p.runCtx = ctx
	p.stages, err = util.ParseConfigToStages(p, p.config.Stages, p.stagesFn...)
	if err != nil {
		p.err = err
		return p.StateSet(err, true)
	}
	//初始化日志
	util.SystemLog(ctx, fmt.Sprintf("流水线%s 开始 \n", p.GetID()))
	//环境准备
	executorList := []pipeline.Executor{
		executor.NewK8sExecutor(p),
		executor.NewLocalExecutor(p),
	}
	defer func() {
		if p.config.Config.K8SExecutor.IsKeepPod {
			return
		}
		defer util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), p), fmt.Sprintf("%s清理环境结束\n", p.id))
		for _, item := range executorList {
			err = item.Destruction(ctx)
			if err != nil {
				util.SystemLog(ctx, fmt.Sprintf("流水线日志信息：%s\n", err))
			}
		}
	}()
	for _, item := range executorList {
		err := item.Prepare(ctx)
		if err != nil {
			util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), p), fmt.Sprintf("环境准备失败：%s\n", err.Error()))
			return p.StateSet(err, false)
		}
	}
	defer p.runCancel()
	for _, item := range p.stages {
		util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), p), fmt.Sprintf("流水线%s 开始运行stage %s\n", p.GetID(), item.GetName()))
		err := item.Start(ctx)
		util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), p), fmt.Sprintf("流水线%s 结束stage %s\n", p.GetID(), item.GetName()))
		if errors.Is(err, pipeline.ErrTimeoutOrCancel) && !p.isCancel {
			return p.StateSet(errors.Wrapf(err, "执行超时，最大执行时间：%d", p.config.Config.Timeout), true)
		}
		if err != nil {
			return p.StateSet(err, true)
		}
		p.StateSet(nil, false)
	}
	p.StateSet(nil, true)
	return nil
}

func (p *PipelineImpl) RegisterImpl(ctx context.Context, impls ...interface{}) {
	for _, item := range impls {
		if log, ok := item.(pipeline.Log); ok {
			p.log = log
		}
	}
}

// StateSet 设置状态 isFinish 是否是正常结束的
func (p *PipelineImpl) StateSet(err error, isFinish bool) error {
	defer p.Notify()
	p.err = err
	util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), p), p.err)
	if p.isCancel || util.IsTerminatorErr(err) {
		util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), p), "流水线状态变更：%s -> %s", p.GetStatus(), pipeline.StatusTerminate)
		p.SetStatus(pipeline.StatusTerminate)
		return nil
	}
	if p.err != nil {
		util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), p), "流水线状态变更：%s -> %s", p.GetStatus(), pipeline.StatusFailed)
		p.SetStatus(pipeline.StatusFailed)
		return err
	}

	if isFinish {
		util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), p), "流水线状态变更：%s -> %s", p.GetStatus(), pipeline.StatusSuccess)
		p.SetStatus(pipeline.StatusSuccess)
		return nil
	}
	util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), p), "流水线状态变更：%s -> %s", p.GetStatus(), pipeline.StatusRunning)
	p.SetStatus(pipeline.StatusRunning)
	return nil
}

// getParam 获取参数
func (p *PipelineImpl) getParam() map[string]interface{} {
	data := p.config.Params.Data
	if data == nil {
		return map[string]interface{}{}
	}
	return data
}

// Notify 流水线变动通知
func (p *PipelineImpl) Notify() {
	//收集日志
	logStr := ""
	if p.isCancel {
		util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), p), "任务被主动终止!\n")
	}
	for _, item := range p.GetStages() {
		for _, step := range item.GetSteps() {
			status := step.GetStatus()
			if status == pipeline.StatusRunning || step.GetLog() != "" {
				// util.UpdateLog(pipeline.ContextWithPipelineImpl(context.Background(), p), p, step.GetID(), status, step.GetLog())
			}
			logStr += step.GetLog()
		}
		for _, stage := range item.GetStages() {
			for _, step := range stage.GetSteps() {
				status := step.GetStatus()
				if status == pipeline.StatusRunning || step.GetLog() != "" {
					// util.UpdateLog(pipeline.ContextWithPipelineImpl(context.Background(), p), p, step.GetID(), status, step.GetLog())
				}
				logStr += step.GetLog()
			}
		}
	}
	sendListen := func() {
		defer p.listeningLock.Unlock()
		p.listeningLock.Lock()
		if !p.listeningCloseed {
			p.listening <- struct{}{}
		}
	}
	sendListen()
}

// SetMetadata 设置metadata
func (p *PipelineImpl) SetMetadata(key string, value interface{}) {
	p.metadata.Store(key, value)
}

// SetStatus 设置流水线状态
func (p *PipelineImpl) SetStatus(status string) {
	//如果状态未取消不能在设置为超时
	p.status = status
	if p.GetParentID() == "" {
		// util.UpdateLog(pipeline.ContextWithPipelineImpl(context.Background(), p), p, p.GetID(), status, util.UpdateState)
	}
	//如果是子流水线需要将最后一个节点进行状态设置
	//条件：如果是UNKONE的话并且设置状态为失败就把最后一个节点设置为失败
	if p.GetParentID() != "" {
		stages := p.GetStages()
		if stages[len(stages)-1].GetStatus() == pipeline.StageStatusUnknown && status == pipeline.StageStatusFailed {
			stages[len(stages)-1].SetStatus(status)
		}
		p.stages = stages
	}
}

// Done 流水线结束回调
func (p *PipelineImpl) Done() <-chan struct{} {
	return p.done
}

func (p *PipelineImpl) doneFn() {
	if util.InArray(p.status, []string{
		pipeline.StatusTerminate,
		pipeline.StatusFailed,
		pipeline.StatusSuccess,
	}) {
		p.onceDone.Do(func() {
			util.SystemLog(pipeline.ContextWithPipelineImpl(context.Background(), p), "流水线结束，通知通知外部channal")
			close(p.done)
		})
	}
}

func (p *PipelineImpl) clean() {
	listenClose := func() {
		defer p.listeningLock.Unlock()
		p.listeningLock.Lock()
		p.listeningCloseed = true
		close(p.listening)
	}
	p.doneFn()
	listenClose()
	//通知runtime
	p.GetRuntime().Notify(true)
}
