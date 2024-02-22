package executor

import (
	"sync"
	"time"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/spf13/cast"
	v1 "k8s.io/api/core/v1"
)

// AdditionSidecarToPod 附加sidecar到容器
func AdditionSidecarToPod(p pipeline.PipelineOperator, pod *v1.PodSpec) {
	config := p.GetConfig()
	//pod添加公共sidecar挂载目录
	sidecarVolumnName := "pipeline-sidecar-share"
	publicVolumnMount := v1.VolumeMount{
		Name:      sidecarVolumnName,
		MountPath: util.GetPublicWorkDir(),
	}
	pod.Volumes = append(pod.Volumes, v1.Volume{
		Name: sidecarVolumnName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	})
	//pod中其他的容器也添加上共享挂载
	containers := []v1.Container{}
	containersExists := map[string]bool{}
	for _, item := range pod.Containers {
		if containersExists[item.Name] {
			continue
		}
		containersExists[item.Name] = true
		item.VolumeMounts = append(item.VolumeMounts, publicVolumnMount)
		containers = append(containers, item)
	}
	pod.Containers = containers
	//pod添加sidecar 容器并挂载公共共享目录
	// 获取当前时间
	now := time.Now()
	// 构造明天凌晨4点的时间
	tomorrow := now.Add(24 * time.Hour) // 加一天，即明天
	targetTime := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 4, 0, 0, 0, tomorrow.Location())
	// 计算时间差
	duration := targetTime.Sub(now)
	// 获取时间差的秒数
	sleepTime := cast.ToString(int(duration.Seconds()))
	pod.Containers = append(pod.Containers, v1.Container{
		Name:  "pipeline-sidecar",
		Image: config.Config.K8SExecutor.SidecarImage,
		Env: []v1.EnvVar{
			{
				Name:  "TIMEOUT",
				Value: sleepTime,
			},
		},
		VolumeMounts: []v1.VolumeMount{
			publicVolumnMount,
		},
		Ports: []v1.ContainerPort{
			{
				ContainerPort: 9999,
			},
		},
	})
	//根据配置添加pod污点数据
	if config.Config.K8SExecutor.PodsNodeSelectorKey != "" && config.Config.K8SExecutor.PodsNodeSelectorValue != "" {
		pod.NodeSelector = map[string]string{
			config.Config.K8SExecutor.PodsNodeSelectorKey: config.Config.K8SExecutor.PodsNodeSelectorValue,
		}
		pod.Tolerations = []v1.Toleration{
			{
				Key:      config.Config.K8SExecutor.PodsNodeSelectorKey,
				Value:    config.Config.K8SExecutor.PodsNodeSelectorValue,
				Operator: v1.TolerationOpEqual,
				Effect:   v1.TaintEffectNoSchedule,
			},
		}
	}
}

// MergePodSpec 合并podspec
func MergePodSpec(pod *v1.PodSpec, pod2 *v1.PodSpec) {
	pod.Volumes = append(pod.Volumes, pod2.Volumes...)
	pod.Containers = append(pod.Containers, pod2.Containers...)
	pod.InitContainers = append(pod.InitContainers, pod2.InitContainers...)
}

type K8sExecutorState struct {
	data map[string]K8sExecutorStateItem
	lock sync.Mutex
}

type K8sExecutorStateItem struct {
	Ids        map[string]bool
	PodName    string
	LiveTime   int64
	CreateTime int64
}

var k8sExecutorStateOnce sync.Once
var k8sExecutorState *K8sExecutorState

func NewK8sExecutorState() *K8sExecutorState {
	k8sExecutorStateOnce.Do(func() {
		k8sExecutorState = &K8sExecutorState{
			data: map[string]K8sExecutorStateItem{},
			lock: sync.Mutex{},
		}
	})
	return k8sExecutorState
}

func (kes *K8sExecutorState) CanCleanup(podName string) bool {
	kes.lock.Lock()
	defer kes.lock.Unlock()
	if _, ok := kes.data[podName]; !ok {
		return false
	}
	if len(kes.data[podName].Ids) == 0 && time.Now().Unix()-kes.data[podName].CreateTime > kes.data[podName].LiveTime {
		//如果这个pod已经没有流水线使用,并且已经超过生存时间，就可以删除对应的键位
		delete(kes.data, podName)
		return true
	}
	return false
}

func (kes *K8sExecutorState) AddPipeline(podName string, pipelineID string, liveTime int64) {
	kes.lock.Lock()
	defer kes.lock.Unlock()
	if item, ok := kes.data[podName]; ok {
		item.Ids[pipelineID] = true
		kes.data[podName] = item
		return
	}
	item := K8sExecutorStateItem{
		Ids: map[string]bool{
			pipelineID: true,
		},
		PodName:    podName,
		LiveTime:   liveTime,
		CreateTime: time.Now().Unix(),
	}
	kes.data[podName] = item
	return
}

func (kes *K8sExecutorState) RemovePipeline(podName string, pipelineID string) {
	kes.lock.Lock()
	defer kes.lock.Unlock()
	if item, ok := kes.data[podName]; ok {
		delete(item.Ids, pipelineID)
		kes.data[podName] = item
	}
}

func (kes *K8sExecutorState) Data() map[string]K8sExecutorStateItem  {
	kes.lock.Lock()
	defer kes.lock.Unlock()
	return kes.data
}
