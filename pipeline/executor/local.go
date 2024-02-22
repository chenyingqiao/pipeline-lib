package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/config"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/pkg/errors"
)

//LocalExecutor 本地执行者
type LocalExecutor struct {
	config config.Pipeline
	p pipeline.PipelineOperator
}

//NewLocalExecutor 执行本地executor
func NewLocalExecutor(p pipeline.PipelineOperator) *LocalExecutor {
	result := &LocalExecutor{
		config: p.GetConfig(),
		p: p,
	}
	return result
}

//Do 执行内容
func (l *LocalExecutor) Do(ctx context.Context, param pipeline.ExecutorParam) <-chan pipeline.ExecutorResult {
	//执行命令
	result := make(chan pipeline.ExecutorResult)
	go func() {
		defer close(result)
		l.generatorFile(ctx, result, param)
		l.executorSh(ctx, result, param)
	}()

	return result
}

func (l *LocalExecutor) workDir(param pipeline.ExecutorParam) string {
	return util.GetStepWorkdirWithSys(param.GetStep())
}

func (l *LocalExecutor) generatorFile(ctx context.Context, result chan pipeline.ExecutorResult, param pipeline.ExecutorParam) {
	if param.GetRuntimeInfo() == nil {
		return
	}
	if _, ok := param.GetRuntimeInfo().(LocalRuntimeInfo); !ok {
		result <- Result{Err: errors.Wrap(errors.New("runtime info is invalid"), "")}
		return
	}
	runtimeInfo := param.GetRuntimeInfo().(LocalRuntimeInfo)
	os.MkdirAll(l.workDir(param), 0777)
	for _, item := range runtimeInfo.Files {
		os.WriteFile(fmt.Sprintf("%s/%s", l.workDir(param), item.Path), []byte(item.Content), 0777)
	}
	return
}

//Prepare 环境准备
func (l *LocalExecutor) Prepare(ctx context.Context) error {
	return nil
}

//Destruction 销毁
func (l *LocalExecutor) Destruction(ctx context.Context) error {
	return nil
}

func (l *LocalExecutor) executorSh(ctx context.Context, result chan pipeline.ExecutorResult, param pipeline.ExecutorParam) {
	workDir := l.workDir(param)
	isTimeout := false
	cmdStr := fmt.Sprintf("cd %s && %s", workDir, param.GetCmd())
	//创建对应的工作目录
	os.MkdirAll(workDir, 0777)
	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	stdOut, err := cmd.StdoutPipe()
	if err != nil {
		result <- Result{Err: errors.Wrap(err, "")}
		return
	}
	stdErr, err := cmd.StderrPipe()
	if err != nil {
		result <- Result{Err: errors.Wrap(err, "")}
		return
	}

	go func() {
		//自动结束任务
		for {
			select {
			case <-ctx.Done():
				if cmd.Process != nil && cmd.Process.Pid > 0 {
					syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					isTimeout = true
					return
				}
				isTimeout = true
			}
		}
	}()

	err = cmd.Start()
	if err != nil {
		result <- Result{Err: errors.Wrap(err, "")}
		return
	}

	executorRes := Result{
		Success: util.Reader(stdOut),
		Error:   util.Reader(stdErr),
	}

	if err := cmd.Wait(); err != nil {
		if isTimeout {
			executorRes.Err = pipeline.ErrTimeoutOrCancel
		} else {
			executorRes.Err = errors.Wrap(err, util.Reader(stdOut)+util.Reader(stdErr))
		}
		result <- executorRes
		return
	}
	if cmd.ProcessState.ExitCode() != 0 {
		if isTimeout {
			executorRes.Err = pipeline.ErrTimeoutOrCancel
		} else {
			executorRes.Err = fmt.Errorf("程序错误退出，exit code: %d", cmd.ProcessState.ExitCode())
		}
		return
	}
	result <- executorRes
	return
}
