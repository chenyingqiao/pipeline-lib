package stages

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/config"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sanity-io/litter"
	"github.com/spf13/cast"
	"github.com/thoas/go-funk"
	"golang.org/x/sync/errgroup"
)

const StageAgentType = "agent"

// Stage k8s自定义执行stage
type Agent struct {
	Stage
	token string
}

type PipelineStateStage struct {
	ID    string `json:"id"`
	State string `json:"state"`
}
type PipelineStateResult struct {
	Stages []PipelineStateStage `json:"stages"`
	State  string               `json:"state"`
}

func (p *PipelineStateResult) IsFinished(stageID string) (bool, error) {
	for _, item := range p.Stages {
		if item.ID != stageID {
			continue
		}
		if item.State == pipeline.StatusFailed {
			return true, pipeline.ErrRemoteExecuteFail
		}
		if item.State == pipeline.StatusSuccess || item.State == pipeline.StatusTerminate {
			return true, nil
		}
	}
	return false, nil
}

func (p *PipelineStateResult) IsStarted() bool {
	isStarted := false
	for _, item := range p.Stages {
		if item.State != pipeline.StageStatusUnknown {
			isStarted = true
		}
	}
	return isStarted
}

// NewStage 实例化stage
func NewAgent(p pipeline.PipelineOperator, data interface{}) (pipeline.StageOperator, error) {
	//参数校验
	if _, ok := data.(map[string]interface{}); !ok {
		return nil, errors.New("data type error: need map[string]string")
	}
	dataArr := data.(map[string]interface{})
	err := validation(dataArr)
	if err != nil {
		return nil, err
	}

	//为stage和step生成id
	addIDField(dataArr)
	stage := Agent{}
	stage.id = cast.ToString(dataArr["id"])
	stage.name = cast.ToString(dataArr["name"])
	stage.parallelism = cast.ToBool(dataArr["parallelism"])
	stage.termination = cast.ToBool(dataArr["termination"])
	stage.steps = []pipeline.StepOperator{}
	stage.stype = cast.ToString(dataArr["type"])
	stage.token = cast.ToString(dataArr["token"])
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
	if stage.stype != StageAgentType {
		return nil, pipeline.ErrStageTypeNotMatch
	}
	return &stage, nil
}

func (kcs *Agent) remoteExecInRetry(ctx context.Context, p pipeline.PipelineOperator, s pipeline.StageOperator) error {
	//获取kcs的id为子流水线id
	childerID := uuid.NewString()
	defer func() {
		//如果stage是被终止的状态就请求终止agent的任务
		if s.GetStatus() == pipeline.StageStatusTerminate {
			kcs.terminationPipeline(ctx, p, s)
		}
	}()

	err := kcs.retry.startRetry(ctx, p, s, func(ctx context.Context) error {
		err := kcs.remoteExec(ctx, p, s, childerID)
		if util.IsTerminatorErr(err) {
			return pipeline.ErrTimeoutOrCancel
		}
		return err
	})
	return err
}

func (kcs *Agent) terminationPipeline(ctx context.Context, p pipeline.PipelineOperator, s pipeline.StageOperator) {
	resp, err := resty.NewWithClient(http.DefaultClient).
		SetTimeout(time.Second*2).
		SetRetryCount(10).
		SetRetryWaitTime(2*time.Second).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			return !r.IsSuccess()
		}).
		R().
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]interface{}{
			"ids": []string{p.GetID()},
		}).
		SetAuthToken(p.GetConfig().Config.AgentStage.Token).
		Post(p.GetConfig().Config.AgentStage.TerminalURL)
	if err != nil {
		util.SystemLog(ctx, "远程终止节点失败，请重试:%s %s", err.Error(), string(resp.Body()))
	}
	if resp.IsError() {
		util.SystemLog(ctx, "请求错误: %s %s", p.GetConfig().Config.AgentStage.TerminalURL, string(resp.Body()))
	}
	s.SetStatus(pipeline.StageStatusTerminate)
}

func (kcs *Agent) executorPipeline(ctx context.Context, p pipeline.PipelineOperator, s pipeline.StageOperator, pConfig config.Pipeline) error {
	//对子流水线的主要id进行特殊处理，不让他和stage的id一样
	executorBody := map[string]interface{}{
		"id":        pConfig.ID,
		"parent_id": pConfig.PID,
		"token":     funk.Get(s.GetConfig(), "token"),
		"config":    pConfig,
	}
	jsonExecuteBody, _ := json.Marshal(executorBody)
	util.SystemLog(ctx, "发起远程agent任务，配置：%s", jsonExecuteBody)
	resp, err := resty.NewWithClient(http.DefaultClient).
		SetRetryCount(10).
		SetRetryWaitTime(2 * time.Second).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			return !r.IsSuccess()
		}).
		R().
		SetAuthToken(pConfig.Config.AgentStage.Token).
		SetBody(executorBody).
		Post(pConfig.Config.AgentStage.ExecURL)
	if err != nil {
		util.SystemLog(ctx, "远程执行节点失败，请重试:%s %s", err.Error(), string(resp.Body()))
		return errors.Wrap(err, "远程执行节点失败，请重试!")
	}
	if resp.IsError() {
		return errors.New("请求错误:" + pConfig.Config.AgentStage.ExecURL + string(resp.Body()))
	}
	s.SetStatus(pipeline.StageStatusRunning)
	return nil
}

func (kcs *Agent) fetchPipelineStatusResult(ctx context.Context, pConfig config.Pipeline) (PipelineStateResult, error) {
	pipelineStateResult := PipelineStateResult{}
	resp, err := resty.NewWithClient(http.DefaultClient).
		SetRetryCount(10).
		SetRetryWaitTime(2 * time.Second).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			return !r.IsSuccess()
		}).
		R().
		SetContext(ctx).
		SetAuthToken(pConfig.Config.AgentStage.Token).
		Get(pConfig.Config.AgentStage.StateURL)
	if err != nil {
		return pipelineStateResult, errors.Wrap(err, "查询流水线执行状态失败!")
	}
	if resp.StatusCode() == http.StatusNotFound {
		return pipelineStateResult, errors.New("访问文件404:%s" + pConfig.Config.AgentStage.StateURL)
	}
	if resp.IsError() {
		return pipelineStateResult, errors.New("流水线状态请求错误:" + pConfig.Config.AgentStage.StateURL + string(resp.Body()))
	}
	err = json.Unmarshal(resp.Body(), &pipelineStateResult)
	if err != nil {
		return pipelineStateResult, errors.Wrap(err, "状态接口反回的数据格式错误，需要为标准化的json!")
	}
	return pipelineStateResult, nil
}

func (kcs *Agent) fetchStatus(ctx context.Context, p pipeline.PipelineOperator, s pipeline.StageOperator, pConfig config.Pipeline) error {
	for {
		kcs.p.Notify()
		time.Sleep(time.Second * 3)
		pipelineStateResult, err := kcs.fetchPipelineStatusResult(ctx, pConfig)
		if util.IsTerminatorErr(err) {
			return pipeline.ErrTimeoutOrCancel
		}
		if err != nil {
			util.SystemLog(ctx, "获取流水线的stages失败:%s", err.Error())
			continue
		}
		statusMap := funk.Map(pipelineStateResult.Stages, func(x PipelineStateStage) (string, string) {
			return x.ID, x.State
		}).(map[string]string)
		notStageStatus := statusMap[s.GetID()]
		if notStageStatus == "" {
			continue
		}
		if notStageStatus == pipeline.StatusSuccess {
			return nil
		}
		if notStageStatus == pipeline.StatusTerminate {
			util.SystemLog(ctx, "远程流水线运行失败：%s", litter.Sdump(statusMap))
			return pipeline.ErrTimeoutOrCancel
		}
		if notStageStatus == pipeline.StatusFailed {
			util.SystemLog(ctx, "远程流水线运行失败：%s", litter.Sdump(statusMap))
			return pipeline.ErrRemoteExecuteFail
		}
	}
}

func (kcs *Agent) metadata(ctx context.Context, p pipeline.PipelineOperator, s pipeline.StageOperator, pConfig config.Pipeline) (map[string]interface{}, error) {
	// 同步metadata数据
	metadata := map[string]interface{}{}
	resp, err := resty.NewWithClient(http.DefaultClient).
		SetRetryCount(10).
		SetRetryWaitTime(2 * time.Second).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			return !r.IsSuccess()
		}).
		R().
		SetContext(ctx).
		SetAuthToken(pConfig.Config.AgentStage.Token).
		Get(pConfig.Config.AgentStage.MetadataURL)
	if err != nil {
		return nil, errors.Wrap(err, "获取流水线metadata失败!")
	}
	if err != nil {
		return nil, errors.Wrap(err, "获取流水线metadata失败!")
	}
	if resp.IsError() {
		return nil, errors.New("流水线metadata请求错误:" + string(resp.Body()))
	}
	err = json.Unmarshal(resp.Body(), &metadata)
	if err != nil {
		return nil, err
	}
	for k, v := range metadata {
		p.SetMetadata(k, v)
	}
	return metadata, nil
}

func (kcs *Agent) remoteExec(ctx context.Context, p pipeline.PipelineOperator, s pipeline.StageOperator, childerID string) error {
	//组织新的子yaml
	pConfig := p.GetConfig()
	sConfig := s.GetConfig()
	sConfig["type"] = ""
	pConfig.Stages = []interface{}{
		sConfig,
	}
	//打印日志，子流水线关系数据
	id := cast.ToString(sConfig["id"])
	name := cast.ToString(sConfig["name"])
	util.SystemLog(ctx, "父亲流水线id: %s 子流水线id:%s 子节点id:%s  %s", p.GetID(), childerID, id, name)
	//附值新的id并且指定父id
	pConfig.ID = childerID
	pConfig.PID = p.GetID()
	//更新metadata
	metadata, err := kcs.metadata(ctx, p, s, p.GetConfig())
	if err == nil {
		metadataArr := []config.Metadata{}
		for k, v := range metadata {
			metadataArr = append(metadataArr, config.Metadata{
				Key:   k,
				Value: v,
			})
		}
		pConfig.Metadata = metadataArr
	}

	//判断节点是否是已经结束了，如果结束了就直接跳过执行
	pipelineStatusResult, err := kcs.fetchPipelineStatusResult(ctx, pConfig)
	if err != nil {
		util.SystemLog(ctx, "恢复流水线时获取状态错误:%s", err.Error())
	}
	if isFinished, err := pipelineStatusResult.IsFinished(kcs.id); isFinished {
		return err
	}
	//请求执行流水线
	err = kcs.executorPipeline(ctx, p, s, pConfig)
	if err != nil {
		util.SystemLog(ctx, "运行流水线失败:%s", err.Error())
		return err
	}

	//轮询流水线运行状态，直到结束状态
	err = kcs.fetchStatus(ctx, p, s, pConfig)
	if err != nil {
		util.SystemLog(ctx, "拉取流水线状态失败:%s", err.Error())
		return err
	}
	_, err = kcs.metadata(ctx, p, s, pConfig)
	if err != nil {
		util.SystemLog(ctx, "拉取metadata失败:%s", err.Error())
		return err
	}
	return nil
}

// Start 执行step
func (kcs *Agent) Start(ctx context.Context) error {
	defer func() {
		kcs.isStarted = false
	}()
	if len(kcs.GetStages()) != 0 {
		if kcs.parallelism {
			//并行
			ewg := errgroup.Group{}
			for _, stage := range kcs.GetStages() {
				stageItem := stage
				ewg.Go(func() error {
					err := kcs.remoteExecInRetry(ctx, kcs.p, stageItem)
					if err != nil {
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
				if err := kcs.remoteExecInRetry(ctx, kcs.p, stage); err != nil {
					kcs.err = err
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
		err := kcs.remoteExecInRetry(ctx, kcs.p, kcs)
		if err != nil {
			kcs.err = err
			if !kcs.termination && !errors.Is(err, pipeline.ErrTimeoutOrCancel) {
				return nil
			}
			return err
		}
		kcs.SetStatus(pipeline.StageStatusSuccess)
	}
	return nil
}
