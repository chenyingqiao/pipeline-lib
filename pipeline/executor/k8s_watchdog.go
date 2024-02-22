package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/chenyingqiao/pipeline-lib/runtime/k8s"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func RunWatchDog(ctx context.Context, cli *kubernetes.Clientset) {
	go func() {
		informers := initK8sInformer(ctx, cli)
		stopCh := make(chan struct{})
		go informers.Run(stopCh)
		cache.WaitForCacheSync(stopCh, informers.HasSynced)
		//启动pod主动清理: 每30秒清理一次
		go crontabToCleanUp(ctx, cli, informers)
		<-stopCh
	}()
}

func initK8sInformer(ctx context.Context, cli *kubernetes.Clientset) cache.SharedIndexInformer {
	informerFactory := informers.NewSharedInformerFactory(cli, 0)
	podInformer := informerFactory.Core().V1().Pods().Informer()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if podObj, ok := obj.(*corev1.Pod); ok {
				if _, has := podObj.ObjectMeta.Labels[k8s.PodLableVersion]; has {
					util.SystemLog(ctx, "RunWatchDog  添加pod: %s\n", podObj.Name)
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if podObj, ok := newObj.(*corev1.Pod); ok {
				if _, has := podObj.ObjectMeta.Labels[k8s.PodLableVersion]; has {
					util.SystemLog(ctx, "更新pod: %s status: %s\n", podObj.Name, podObj.Status.Phase)
					//如果pod状态更新为失败或者是删除状态就将pod删除
					if podObj.Status.Phase != corev1.PodFailed && podObj.Status.Phase != corev1.PodSucceeded {
						return
					}
					err := k8s.NewPod().Delete(ctx, cli, podObj.Namespace, podObj.Name)
					if err != nil {
						util.SystemLog(ctx, "RunWatchDog  删除pod {%s.%s} 失败：%s\n", podObj.Namespace, podObj.Name, err.Error())
						return
					}
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			if podObj, ok := obj.(*corev1.Pod); ok {
				if _, has := podObj.ObjectMeta.Labels[k8s.PodLableVersion]; has {
					util.SystemLog(ctx, "RunWatchDog  删除pod: %s status: %s", podObj.Name, podObj.Status.Phase)
				}
			}
		},
	})

	return podInformer
}

func crontabToCleanUp(ctx context.Context, cli *kubernetes.Clientset, informers cache.SharedIndexInformer) {
	timer := time.NewTicker(60 * time.Second)
	for {
		select {
		case <-timer.C:
			//通过判断pod是否已经结束了，如果是已经结束了的pod就进行清除
			pipelinexPods := getPodsByLabel(informers.GetIndexer(), labels.SelectorFromSet(labels.Set{k8s.PodLableVersion: pipeline.VERSION}), false)
			for _, pod := range pipelinexPods {
				err := k8s.NewPod().Delete(ctx, cli, pod.Namespace, pod.Name)
				if err != nil {
					util.SystemLog(ctx, "RunWatchDog.Faild  删除pod {%s.%s} 失败：%s\n", pod.Namespace, pod.Name, err.Error())
				}
			}

			//清理pod到期pod
			workloadCleanUp(ctx, cli, informers)
		case <-ctx.Done():
			timer.Stop()
		}
	}
}

func workloadCleanUp(ctx context.Context, cli *kubernetes.Clientset, informers cache.SharedIndexInformer) error {
	state := NewK8sExecutorState()
	pipelinexPods := getPodsByLabel(informers.GetIndexer(), labels.SelectorFromSet(labels.Set{k8s.PodLableVersion: pipeline.VERSION}), true)
	for _, pod := range pipelinexPods {
		delPod := func(ctx context.Context, cli *kubernetes.Clientset, pod *v1.Pod) {
			lockid := fmt.Sprintf("%s.%s", pod.Namespace, pod.Name)
			util.NewLocker().Lock(lockid)
			defer util.NewLocker().Unlock(lockid)
			if state.CanCleanup(pod.Name) && !hasOtherMarsAgent(informers.GetIndexer()) {
				util.SystemLog(ctx, "RunWatchDog.workloadCleanUp  删除pod %s %s\n", pod.Namespace, pod.Name)
				err := k8s.NewPod().Delete(ctx, cli, pod.Namespace, pod.Name)
				if err != nil {
					util.SystemLog(ctx, "RunWatchDog.workloadCleanUp  删除pod {%s.%s} 失败：%s\n", pod.Namespace, pod.Name, err.Error())
				}
			}
		}
		delPod(ctx, cli, pod)
	}

	//打印工作负载管理器
	stateJson, err := json.Marshal(state.Data())
	if err != nil {
		util.SystemLog(ctx, "RunWatchDog.state 运行状态 %s", string(stateJson))
	}
	return nil
}

func hasOtherMarsAgent(indexer cache.Indexer) bool {
	hasOtherMarsAgent := 0
	// 获取所有存储在Indexer中的Pod对象
	podList := indexer.List()

	// 遍历所有Pod对象，并检查它们的标签是否匹配
	for _, obj := range podList {
		pod := obj.(*corev1.Pod)
		if strings.HasPrefix(pod.Name, "mars-agent-") {
			hasOtherMarsAgent += 1
		}
		if hasOtherMarsAgent > 1 {
			return true
		}
	}

	return hasOtherMarsAgent > 1
}

// 按标签从Indexer中获取Pod对象
func getPodsByLabel(indexer cache.Indexer, labelSelector labels.Selector, isRunning bool) []*corev1.Pod {
	var matchingPods []*corev1.Pod

	// 获取所有存储在Indexer中的Pod对象
	podList := indexer.List()

	// 遍历所有Pod对象，并检查它们的标签是否匹
	for _, obj := range podList {
		pod := obj.(*corev1.Pod)
		if !isRunning {
			if !(pod.Status.Phase == v1.PodFailed || pod.Status.Phase == v1.PodSucceeded) {
				continue
			}
		} else {
			if pod.Status.Phase != v1.PodRunning {
				continue
			}
		}
		if labelSelector.Matches(labels.Set(pod.Labels)) {
			matchingPods = append(matchingPods, pod)
		}
	}

	return matchingPods
}
