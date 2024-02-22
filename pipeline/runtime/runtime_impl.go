package runtime

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/chenyingqiao/pipeline-lib/pipeline/pipe"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/stages"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/pkg/errors"
	"github.com/sanity-io/litter"
)

var runtimeImpl *RuntimeImpl
var runtimeOnce sync.Once

// RuntimeImpl 运行时实现
type RuntimeImpl struct {
	pool             sync.Map //string:pipeline.PipelineOperator
	doneChan         chan struct{}
	onceLock         sync.Once
	runtimeCtx       context.Context
	runtimeCtxCancel context.CancelFunc
}

// NewRuntimeImpl 实例化pipeline
func NewRuntimeImpl() *RuntimeImpl {
	runtimeOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		runtimeImpl = &RuntimeImpl{
			doneChan:         make(chan struct{}),
			onceLock:         sync.Once{},
			runtimeCtx:       ctx,
			runtimeCtxCancel: cancel,
		}
	})
	return runtimeImpl
}

// Get 获取流水线状态
func (r *RuntimeImpl) Get(id string) (pipeline.Pipeline, error) {
	if p, ok := r.pool.Load(id); ok {
		return p.(pipeline.Pipeline), nil
	}
	return nil, errors.New("pipeline is not found")
}

// Cancel 取消运行中的流水线
func (r *RuntimeImpl) Cancel(ctx context.Context, id string) error {
	if p, ok := r.pool.Load(id); ok {
		p.(pipeline.PipelineOperator).Cancel()
		ctx := pipeline.ContextWithPipelineImpl(context.Background(), p.(pipeline.PipelineOperator))
		util.SystemLog(ctx, "终止流水线：%s", id)
		return nil
	}
	return errors.New("pipeline is not found")
}

// RunAsync 运行异步流水线
func (r *RuntimeImpl) RunAsync(ctx context.Context, id string, config string, fn pipeline.PipelineListeningFn, implement ...interface{}) (pipeline.Pipeline, error) {
	stages := []pipeline.NewStage{
		stages.NewStage,
		stages.NewAgent,
	}

	p, err := pipe.NewPipelineImpl(ctx, r, id, config, stages...)
	if err != nil {
		return nil, err
	}
	r.pool.Store(id, p)
	p.RegisterImpl(ctx, implement...)
	go func() {
		defer func() {
			v := recover()
			if err, ok := v.(error); ok && err != nil {
				util.SystemLog(pipeline.ContextWithPipelineImpl(ctx, p), "==========流水线%s发生panic==========\n 错误信息\n%s \n堆栈信息：\n%s", p.GetID(), litter.Sdump(v), string(debug.Stack()))
			}
		}()
		p.Listening(fn)
		p.Run(ctx)
	}()
	return p, nil
}

// RunSync 运行同步流水线
func (r *RuntimeImpl) RunSync(ctx context.Context, id string, config string, fn pipeline.PipelineListeningFn, implement ...interface{}) (pipeline.Pipeline, error) {
	stages := []pipeline.NewStage{
		stages.NewStage,
		stages.NewAgent,
	}

	p, err := pipe.NewPipelineImpl(ctx, r, id, config, stages...)
	if err != nil {
		return nil, err
	}
	defer func() {
		v := recover()
		if err, ok := v.(error); ok && err != nil {
			util.SystemLog(pipeline.ContextWithPipelineImpl(ctx, p), "==========流水线%s发生panic==========\n 错误信息\n%s \n堆栈信息：\n%s", p.GetID(), litter.Sdump(v), string(debug.Stack()))
		}
		util.SystemLog(pipeline.ContextWithPipelineImpl(ctx, p), "执行流水线发生异常：%s", litter.Sdump(v))
	}()
	p.RegisterImpl(ctx, implement...)
	p.Listening(fn)
	r.pool.Store(id, p)
	err = p.Run(ctx)
	if err != nil {
		err = fmt.Errorf("%s\n\tmetadata:\n%s", err.Error(), util.FormatMetadata(p.GetMetadata()))
		return nil, err
	}
	return p, nil
}

// Rm 移除流水线记录
func (r *RuntimeImpl) Rm(id string) {
	defer r.pool.Delete(id)
	if p, ok := r.pool.Load(id); ok {
		if _, ok := p.(pipeline.PipelineOperator); !ok {
			return
		}
		ctx := pipeline.ContextWithPipelineImpl(context.Background(), p.(pipeline.PipelineOperator))
		util.SystemLog(ctx, "移除流水线：%s", id)
	}
}

// Ack 应答
func (r *RuntimeImpl) Ack(ctx context.Context, id string, stepID string) error {
	pIface, ok := r.pool.Load(id)
	if !ok {
		return errors.New("pipeline is not found")
	}
	p := pIface.(pipeline.PipelineOperator)
	for _, item := range p.GetStages() {
		if item.GetID() == stepID {
			item.Answer(ctx, true)
		}
		if len(item.GetStages()) > 0 {
			for _, csItem := range item.GetStages() {
				if csItem.GetID() == stepID {
					csItem.Answer(ctx, true)
				}
			}
		}
	}
	return nil
}

// Done 运行时是否已经结束
func (r *RuntimeImpl) Done() chan struct{} {
	//触发done监听的时候需要主动通知一下runtime更新一下状态
	fmt.Printf("runtime 监听Done事件")
	r.Notify(nil)
	return r.doneChan
}

// Notify 触发通知
func (r *RuntimeImpl) Notify(data interface{}) error {
	return r.notifyDone(data)
}

// notifyDone 通知runtime
func (r *RuntimeImpl) notifyDone(data interface{}) error {
	hasPipe := false
	r.pool.Range(func(key, value any) bool {
		hasPipe = true
		return false
	})
	if !hasPipe {
		//关闭通知
		r.onceLock.Do(func() {
			close(r.doneChan)
		})
	}
	return nil
}

func (r *RuntimeImpl) Ctx() context.Context {
	return r.runtimeCtx
}

func (r *RuntimeImpl) StopBackground() {
	r.runtimeCtxCancel()
}
