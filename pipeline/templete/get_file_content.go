package templete

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/executor"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/chenyingqiao/pipeline-lib/runtime/k8s"
	"github.com/pkg/errors"
)

var (
	RuntimeInfoErr = errors.New("runtimeinfo type error")
)

// GetFileContentFunc 获取文件内容函数
type GetFileContentFunc func(ctx context.Context, p pipeline.Pipeline, stage pipeline.Stage, step pipeline.Step, runtimeInfo interface{}, path string) (string, error)

// K8sGetFileContent k8s 获取文件内容
func K8sGetFileContent(ctx context.Context, p pipeline.Pipeline, stage pipeline.Stage, step pipeline.Step, runtimeInfo interface{}, path string) (string, error) {
	if _, ok := runtimeInfo.(executor.K8sRuntimeInfo); !ok {
		return "", RuntimeInfoErr
	}
	runtimeInfoObj := runtimeInfo.(executor.K8sRuntimeInfo)

	c := p.GetConfig()
	cli, config, _ := k8s.GetInClusterClient()
	if c.Config.K8SExecutor.ClientConfig != "" {
		cli, config, _ = k8s.GetOutClusterClient(c.Config.K8SExecutor.ClientConfig)
	}

	successBuffer := bytes.Buffer{}
	errorBuffer := bytes.Buffer{}
	err := k8s.NewExec().RunSh(ctx, cli, config, &k8s.ExecShellFileParam{
		Namespace:     runtimeInfoObj.Namespace,
		PodName:       runtimeInfoObj.PodName,
		ContainerName: runtimeInfoObj.ContainerName,
		Shell:         fmt.Sprintf("cat /tmp/%s/%s && exit 0", p.GetID(), path),
		SuccessWriter: &successBuffer,
		ErrorWriter:   &errorBuffer,
	})
	result := ""
	if err != nil {
		result = err.Error()
	}
	if successBuffer.Len() != 0 {
		result = successBuffer.String()
	}
	if errorBuffer.Len() != 0 {
		result = errorBuffer.String()
	}
	return result, nil
}

// LocalGetFileContent 获取本地文件内容
func LocalGetFileContent(ctx context.Context, p pipeline.Pipeline, stage pipeline.Stage, step pipeline.Step, runtimeInfo interface{}, path string) (string, error) {
	if _, ok := runtimeInfo.(executor.LocalRuntimeInfo); !ok {
		return "", RuntimeInfoErr
	}
	bytes, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", util.GetStepWorkdirWithSys(step), path))
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}
func CopyMetadaFileByPod(ctx context.Context, p pipeline.Pipeline, runtimeInfo interface{}) (string, error) {

	if _, ok := runtimeInfo.(executor.K8sRuntimeInfo); !ok {
		return "", RuntimeInfoErr
	}
	runtimeInfoObj := runtimeInfo.(executor.K8sRuntimeInfo)

	c := p.GetConfig()
	cli, config, _ := k8s.GetInClusterClient()
	if c.Config.K8SExecutor.ClientConfig != "" {
		cli, config, _ = k8s.GetOutClusterClient(c.Config.K8SExecutor.ClientConfig)
	}
	successBuffer := bytes.Buffer{}
	errorBuffer := bytes.Buffer{}
	err := k8s.NewExec().RunSh(ctx, cli, config, &k8s.ExecShellFileParam{
		Namespace:     runtimeInfoObj.Namespace,
		PodName:       runtimeInfoObj.PodName,
		ContainerName: runtimeInfoObj.ContainerName,
		Shell:         fmt.Sprintf("cat /tmp/%s/%s.json  && exit 0", p.GetID(), runtimeInfoObj.ContainerName),
		SuccessWriter: &successBuffer,
		ErrorWriter:   &errorBuffer,
	})
	result := ""
	if err != nil {
		return "", err
	}
	if successBuffer.Len() != 0 {
		result = successBuffer.String()
		return result, nil
	}
	if errorBuffer.Len() != 0 {
		result = errorBuffer.String()
		return "", errors.New(result)
	}
	return result, nil

}
