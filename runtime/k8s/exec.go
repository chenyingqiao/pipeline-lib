package k8s

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/sanity-io/litter"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// Exec 进入容器执行命令
type Exec struct {
	IsTimeout bool
}

// NewExec 执行命令
func NewExec() *Exec {
	return &Exec{}
}

// ExecShellFileParam 执行shell文件的参数
type ExecShellFileParam struct {
	Namespace     string
	PodName       string
	ContainerName string
	Shell         string
	Timeout       time.Duration
	SuccessWriter io.Writer
	ErrorWriter   io.Writer
	HasTTy        bool
	Version       string
}

// RunSh 执行sh文件
func (e *Exec) RunSh(ctx context.Context, cli *kubernetes.Clientset, config *rest.Config, param *ExecShellFileParam) error {
	p := pipeline.GetPipelineImplFromContext(ctx)
	if p != nil {
		confJsonStr, err := json.Marshal(p.GetConfig())
		if err == nil && strings.Contains(string(confJsonStr), "HUAWEI_YUN_DEBUG") {
			param.HasTTy = true
		}
	}
	util.SystemLog(ctx, "执行命令%s：%s\n", param.PodName, litter.Sdump(param))
	req := cli.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(param.PodName).
		Namespace(param.Namespace).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Container: param.ContainerName,
			Command: []string{
				"sh",
				"-c",
				param.Shell,
			},
			Stdin:  true,
			Stdout: true,
			Stderr: true,
			TTY:    param.HasTTy,
		}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		util.SystemLog(ctx, "实例化双向流失败%s：%s\n", param.PodName, err)
		return err
	}
	stdInReader, stdInWriter := io.Pipe()
	streamContext, streamCancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				defer stdInReader.Close()
				defer streamCancel()
				e.IsTimeout = true
				_, err := stdInWriter.Write([]byte{byte(3)})
				if err != nil {
					util.SystemLog(ctx, "触发终止失败: %s", err)
				}
				util.SystemLog(ctx, "触发终止: %s %s %s", param.PodName, param.Shell, err)
				return
			}
		}
	}()
	err = exec.StreamWithContext(streamContext, remotecommand.StreamOptions{
		Stdin:  stdInReader,
		Stdout: param.SuccessWriter,
		Stderr: param.ErrorWriter,
		Tty:    false,
	})
	if err != nil {
		util.SystemLog(ctx, "执行命令失败%s：%s\n", param.PodName, err)
		return err
	}
	util.SystemLog(ctx, "执行命令%s：完成 %s", param.PodName, litter.Sdump(param))
	return nil
}
