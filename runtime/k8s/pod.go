package k8s

import (
	"context"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"time"

	apimachineryErrors "k8s.io/apimachinery/pkg/api/errors" 
	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/pkg/errors"
	"github.com/sanity-io/litter"
	"github.com/spf13/cast"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	PodLableVersion     = "pipelinex-version"
	PodLablePipelineID  = "pipelinex-id"
	PodLablePipelinePID = "pipeline-pid"
)

// Pod k8s pod操作类
type Pod struct{}

// NewPod 添加pod
func NewPod() *Pod {
	return &Pod{}
}

// SimpleV1PodSpac 自定义容器列表
// TODO 这个需要能从外部配置
func (kc *Pod) SimpleV1PodSpac(p pipeline.PipelineOperator, podName string, namespace string, podSpace v1.PodSpec) *v1.Pod {
	defaultPodSpec := v1.PodSpec{
		RestartPolicy: v1.RestartPolicyNever,
		ImagePullSecrets: []v1.LocalObjectReference{
			{
				Name: "",
			},
			{
				Name: "",
			},
		},
		//TODO 需要外部配置
		ServiceAccountName: "",
	}
	if len(podSpace.InitContainers) != 0 {
		defaultPodSpec.InitContainers = podSpace.InitContainers
	}
	if len(podSpace.Containers) != 0 {
		defaultPodSpec.Containers = podSpace.Containers
	}
	if len(podSpace.Volumes) != 0 {
		defaultPodSpec.Volumes = podSpace.Volumes
	}
	if len(podSpace.NodeSelector) != 0 {
		defaultPodSpec.NodeSelector = podSpace.NodeSelector
	}
	if len(podSpace.Tolerations) != 0 {
		defaultPodSpec.Tolerations = podSpace.Tolerations
	}
	return &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				PodLableVersion:     pipeline.VERSION,
				PodLablePipelineID:  p.GetID(),
				PodLablePipelinePID: p.GetParentID(),
			},
		},
		Spec: defaultPodSpec,
	}
}

// SimpleV1Pod 通过imageURL获取一个Pod配置对象
func (kc *Pod) SimpleV1Pod(ctx context.Context, podName, imageURL string) *v1.Pod {
	p := pipeline.GetPipelineImplFromContext(ctx)
	md5.Sum([]byte(imageURL))
	result := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				PodLableVersion: pipeline.VERSION,
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  podName,
					Image: imageURL,
					Command: []string{
						"sleep",
						cast.ToString(p.GetConfig().Config.K8SExecutor.LiveTime),
					},
				},
			},
			ImagePullSecrets: []v1.LocalObjectReference{
				{
					Name: "regsecret-aliyun",
				},
				{
					Name: "regsecret-harbor",
				},
			},
		},
	}
	return result
}

// Create 创建pod
func (kc *Pod) Create(ctx context.Context, cli *kubernetes.Clientset, namespace string, pod *v1.Pod) error {
	pod, err := cli.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if apimachineryErrors.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, err.Error())
	}
	return nil
}

// Delete 删除pod
func (kc *Pod) Delete(ctx context.Context, cli *kubernetes.Clientset, namespace, name string) error {
	err := cli.CoreV1().Pods(namespace).Delete(ctx, name, *metav1.NewDeleteOptions(10))
	if err != nil {
		return errors.Wrap(err, err.Error())
	}
	return nil
}

// Status pod是否存在
func (kc *Pod) Status(ctx context.Context, cli *kubernetes.Clientset, namespace, name string) (v1.PodPhase, bool) {
	pod, err := cli.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", false
	}
	return pod.Status.Phase, true
}

// List 获取pod列表通过命名空间和选项
func (kc *Pod) List(ctx context.Context, cli *kubernetes.Clientset, namespace string, opts metav1.ListOptions) (*v1.PodList, error) {
	pods, err := cli.CoreV1().Pods(namespace).List(ctx, opts)
	if err != nil {
		return nil, err
	}
	return pods, nil
}

// Log 获取日志信息
func (kc *Pod) Log(ctx context.Context, cli *kubernetes.Clientset, namespace, name, container string) string {
	resp := cli.CoreV1().Pods(namespace).GetLogs(name, &v1.PodLogOptions{
		Container: container,
	})
	ioReader, err := resp.Stream(ctx)
	if err != nil {
		return err.Error()
	}
	logBytes, err := ioutil.ReadAll(ioReader)
	if err != nil {
		return err.Error()
	}
	return string(logBytes)
}

// Info 获取pod地址
func (kc *Pod) Info(ctx context.Context, cli *kubernetes.Clientset, namespace, name string) (*v1.Pod, error) {
	pod, err := cli.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pod, nil
}

// ContainerIsReady 判断容器是否已经准备好
func (kc *Pod) ContainerIsReady(ctx context.Context, cli *kubernetes.Clientset, namespace, name string) (error, bool) {
	pod, err := kc.Info(ctx, cli, namespace, name)
	if err != nil {
		return err, true
	}
	if pod.Status.Phase != v1.PodRunning {
		return nil, false
	}
	for _, item := range pod.Status.ContainerStatuses {
		util.SystemLog(ctx, "容器状态：%s %s %s", pod.Name, item.Name, litter.Sdump(pod.Status.ContainerStatuses))
		if !item.Ready && item.State.Running.StartedAt.Unix() < time.Now().Unix() {
			return fmt.Errorf("容器%s未准备好  原因: %s %s %s 日志：%s", item.Name, item.State.Terminated.Reason, pod.Status.Message, pod.Status.Reason, kc.Log(ctx, cli, namespace, name, item.Name)), true
		}
	}
	return nil, true
}

// WaitReason 获取等待原因
func (kc *Pod) WaitReason(ctx context.Context, cli *kubernetes.Clientset, namespace, name string) (string, bool) {
	failStatus := []string{
		"CrashLoopBackOff",
		"InvalidImageName",
		"ImageInspectError",
		"ErrImageNeverPull",
		"ImagePullBackOff",
		"RegistryUnavailable",
		"ErrImagePull",
		"CreateContainerConfigError",
		"CreateContainerError",
		"PreStartContainer",
		"RunContainerError",
		"PostStartHookError",
		"ContainersNotInitialized",
	}

	pod, err := kc.Info(ctx, cli, namespace, name)
	if err != nil {
		return err.Error(), false
	}
	result := ""
	isSuccess := true
	for _, item := range pod.Status.InitContainerStatuses {
		if item.State.Waiting == nil {
			continue
		}
		if util.InArray(item.State.Waiting.Reason, failStatus) {
			isSuccess = false
		}
		if item.State.Waiting.Reason != "" {
			result += fmt.Sprintf("%s 等待原因:%s 详细信息：%s\n", item.Name, item.State.Waiting.Reason, item.State.Waiting.Message)
		}
	}
	for _, item := range pod.Status.ContainerStatuses {
		if item.State.Waiting == nil {
			continue
		}
		if util.InArray(item.State.Waiting.Reason, failStatus) {
			isSuccess = false
		}
		if item.State.Waiting.Reason != "" {
			result += fmt.Sprintf("%s 等待原因:%s 详细信息：%s\n", item.Name, item.State.Waiting.Reason, item.State.Waiting.Message)
		}
	}
	return result, isSuccess
}

// GetFailPodLog 获取失败容器的日志信息
func (kc *Pod) GetFailPodLog(ctx context.Context, cli *kubernetes.Clientset, namespace, name string) string {
	pod, err := kc.Info(ctx, cli, namespace, name)
	if err != nil {
		return err.Error()
	}
	result := ""
	for _, item := range pod.Status.InitContainerStatuses {
		if item.State.Terminated == nil {
			continue
		}
		if item.State.Terminated.ExitCode != 0 {
			result += fmt.Sprintf("\n失败原因:%s\n", item.Name)
			result += kc.Log(ctx, cli, namespace, name, item.Name)
		}
	}
	for _, item := range pod.Status.ContainerStatuses {
		if item.State.Terminated == nil {
			continue
		}
		if item.State.Terminated.ExitCode != 0 {
			result += fmt.Sprintf("\n失败原因:%s\n", item.Name)
			result += kc.Log(ctx, cli, namespace, name, item.Name)
		}
	}
	return result
}
