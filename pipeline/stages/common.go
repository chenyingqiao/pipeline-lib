package stages

import (
	"context"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
)

// Retry 重试
type Retry struct {
	R   bool `yaml:"retry"`
	Ask chan interface{}
}

// RetryCallback 重试逻辑
type RetryCallback func(ctx context.Context) error

// startRetry 重试函数，并且到达一定的次数自动退出，返回最后一次的错误
func (r *Retry) startRetry(ctx context.Context, p pipeline.PipelineOperator, stage pipeline.StageOperator, fn RetryCallback) error {
	suspend := func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return pipeline.ErrTimeoutOrCancel
		case answer := <-r.Ask:
			if cast.ToBool(answer) {
				return nil
			}
			return errors.New("pipeline termination")
		}
	}
	defer util.SystemLog(ctx, "流水线重试结束")
	var err error
	for true {
		err = fn(ctx)
		if !r.R && err != nil && errors.Is(err, pipeline.ErrTimeoutOrCancel) {
			stage.SetStatus(pipeline.StageStatusTerminate)
			return err
		}
		if !r.R && err != nil {
			stage.SetStatus(pipeline.StageStatusFailed)
			return err
		}
		if err == nil {
			stage.SetStatus(pipeline.StageStatusSuccess)
			return nil
		}
		if r.R {
			stage.SetStatus(pipeline.StageStatusPaused)
			p.SetStatus(pipeline.StatusPaused)
			//动态添加一个empty的step
			util.SystemLog(ctx, "流水线挂起")
			err = suspend(ctx)
			if err != nil {
				stage.SetStatus(pipeline.StageStatusFailed)
				p.SetStatus(pipeline.StatusTerminate)
				return err
			}
			stage.SetStatus(pipeline.StageStatusRunning)
			p.SetStatus(pipeline.StatusRunning)
		}
	}
	return nil
}

func (r *Retry) answer(ctx context.Context, data interface{}) {
	r.Ask <- data
}

func addIDField(data map[string]interface{}) {
	if _,ok := data["id"]; !ok {
		data["id"] = uuid.New().String()
	}  
	for key, value := range data {
		if key == "stages" || key == "steps" {
			// 为 stages 和 steps 下的元素添加 id 字段
			list, ok := value.([]interface{})
			if ok {
				for _, item := range list {
					if itemMap, ok := item.(map[string]interface{}); ok {
						if _,ok := itemMap["id"];ok {
							//如果已经存在id了就不进行初始化了
							continue
						}
						itemMap["id"] = uuid.New().String()
						addIDField(itemMap)
					}
				}
			}
		}
	}
}
