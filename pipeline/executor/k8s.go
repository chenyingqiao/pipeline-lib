package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/config"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/chenyingqiao/pipeline-lib/runtime/k8s"
	"github.com/pkg/errors"
	"github.com/sanity-io/litter"
	"github.com/spf13/cast"
	"github.com/thoas/go-funk"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var k8sWatchDogOnceLock sync.Once

// K8sExecutor k8s任务执行者
type K8sExecutor struct {
	config     config.Pipeline
	p          pipeline.PipelineOperator
	hasPrepare bool
	logLock    sync.Mutex
}

// NewK8sExecutor 实例化execute
func NewK8sExecutor(p pipeline.PipelineOperator) *K8sExecutor {
	return &K8sExecutor{
		config:  p.GetConfig(),
		p:       p,
		logLock: sync.Mutex{},
	}
}

// mPodSpecReverse 递归合并pod信息
func (k *K8sExecutor) mPodSpecRecursion(podSpec *v1.PodSpec, stages []pipeline.StageOperator) bool {
	needPod := false
	if len(stages) == 0 {
		return needPod
	}
	for _, stageItem := range stages {
		stype := stageItem.GetConfig()["type"]
		if cast.ToString(stype) != "" {
			continue
		}
		if len(stageItem.GetStages()) > 0 {
			needPod = k.mPodSpecRecursion(podSpec, stageItem.GetStages())
		}
		for _, step := range stageItem.GetSteps() {
			env, envType := step.Env()
			if envType == pipeline.K8s {
				podEnv := util.CastToK8sEnv(env)
				MergePodSpec(podSpec, &podEnv)
				needPod = true
			}
		}
	}
	return needPod
}

// Prepare 环境准备
func (k *K8sExecutor) Prepare(ctx context.Context) error {
	p := k.p
	hasErr := false
	defer func() {
		if !hasErr {
			return
		}
		//如果生成pod有错误就直接调用销毁
		k.Destruction(ctx)
	}()
	//检查pod是否存在并且在运行中
	pod := k8s.NewPod()
	podSpac := v1.PodSpec{}
	needPod := k.mPodSpecRecursion(&podSpac, p.GetStages())
	if !needPod {
		return nil
	}
	//初始化链接
	cli, _, err := k8s.GetInClusterClient()
	if k.config.Config.K8SExecutor.ClientConfig != "" {
		cli, _, err = k8s.GetOutClusterClient(k.config.Config.K8SExecutor.ClientConfig)
	}
	if err != nil {
		hasErr = true
		return err
	}
	//添加sidecar
	AdditionSidecarToPod(p, &podSpac)
	namespace := k.getNamespace()
	podName := k.getPodName()
	//这个是一个原子操作是一个临界区域，如果多个携程对同一个pod进行操作会有竞争需要加锁
	lockid := fmt.Sprintf("%s.%s", namespace, podName)
	util.NewLocker().Lock(lockid)
	defer util.NewLocker().Unlock(lockid)
	//创建环境成功添加到k8s执行器的统计数据中
	NewK8sExecutorState().AddPipeline(podName, k.p.GetID(), int64(k.p.GetConfig().Config.K8SExecutor.LiveTime))
	//添加环境复用锁，防止环境创建逻辑重复进行
	util.SetFirstStageStepInitLog(p, "-----------------------工作负载创建-------------------------\n🛸开始创建环境,工作负载名称: %s.%s", k.getNamespace(), podName)
	err = k.checkPodIsRunning(
		ctx,
		pod,
		cli,
		p,
		pod.SimpleV1PodSpac(
			p,
			podName,
			k.getNamespace(),
			podSpac,
		),
	)
	util.SystemLog(ctx, "初始化k8s执行环境：%spod状态 %v", podName, err)
	if err != nil {
		hasErr = true
		return err
	}
	k.hasPrepare = true
	util.SetFirstStageStepInitLog(p, "🛸初始化k8s执行环境成功")
	return nil
}

// Destruction 销毁环境
func (k *K8sExecutor) Destruction(ctx context.Context) error {
	podName := k.getPodName()
	defer NewK8sExecutorState().RemovePipeline(podName, k.p.GetID())
	if !k.hasPrepare {
		return nil
	}
	//启动看门狗
	k8sWatchDogOnceLock.Do(func() {
		cli, _, err := k8s.GetInClusterClient()
		if k.p.GetConfig().Config.K8SExecutor.ClientConfig != "" {
			cli, _, err = k8s.GetOutClusterClient(k.p.GetConfig().Config.K8SExecutor.ClientConfig)
		}
		if err != nil {
			util.SystemLog(context.Background(), "初始化k8s执行环境：k8s废弃容器清理看门狗初始化失败,err:%s", err.Error())
			return
		}
		RunWatchDog(k.p.GetRuntime().Ctx(), cli)
	})
	//如果是包含beforeAction的pod需要进行销毁
	if k.p.GetConfig().Config.K8SExecutor.Destruction {
		cli, _, err := k8s.GetInClusterClient()
		if k.p.GetConfig().Config.K8SExecutor.ClientConfig != "" {
			cli, _, err = k8s.GetOutClusterClient(k.p.GetConfig().Config.K8SExecutor.ClientConfig)
		}
		if err != nil {
			return err
		}
		//检查pod是否存在并且在运行中
		pod := k8s.NewPod()
		err = pod.Delete(context.Background(), cli, k.getNamespace(), podName)
		if err != nil {
			util.SystemLog(ctx, "pod %s 删除失败 :%s", podName, err.Error())
			return err
		}
	}
	return nil
}

// Do 执行命令
func (k *K8sExecutor) Do(ctx context.Context, param pipeline.ExecutorParam) <-chan pipeline.ExecutorResult {
	//变量初始化
	var runtimeInfo K8sRuntimeInfo
	var ok bool

	//执行
	result := make(chan pipeline.ExecutorResult)
	if runtimeInfo, ok = param.GetRuntimeInfo().(K8sRuntimeInfo); !ok {
		executorRes := Result{
			Err: errors.New("type error: need K8sRuntimeInfo"),
		}
		result <- executorRes
	}
	//获取初始化容器日志信息
	initLog := k.log(ctx, runtimeInfo)
	util.SystemLog(ctx, "初始化k8s执行环境：开始执行命令")
	util.SystemLog(ctx, "初始化k8s执行环境：init容器执行日志 %s", initLog)
	util.SetFirstStageStepInitLog(k.p, "\n\n-----------------------执行任务-------------------------\n🚀前置命令执行日志:\n%s", initLog)
	go func() {
		defer func() {
			close(result)
		}()
		clen := 0
		collectionLogFunc := func() {
			time.Sleep(time.Millisecond * 50)
			if clen == runtimeInfo.SuccessWriter.Len()+runtimeInfo.ErrorWriter.Len() {
				return
			}
			clen = runtimeInfo.SuccessWriter.Len() + runtimeInfo.ErrorWriter.Len()
			executorRes := Result{
				Success: runtimeInfo.SuccessWriter.String(),
				Error:   runtimeInfo.ErrorWriter.String(),
			}
			executorRes.Success = initLog + executorRes.Success
			result <- executorRes
		}
		//日志定时触发输出
		defer collectionLogFunc()
		for {
			select {
			case <-runtimeInfo.Context.Done():
				return
			default:
				collectionLogFunc()
			}
		}
	}()
	go func() {
		defer func() {
			//取消日志输出
			runtimeInfo.Cancel()
		}()
		err := k.execute(ctx, param, &runtimeInfo)
		if err != nil {
			util.SystemLog(ctx, "初始化k8s执行环境：pipelineID: %s pod %s 命令执行错误 %s", k.p.GetID(), runtimeInfo.PodName, err)
			executorRes := Result{
				Success: runtimeInfo.SuccessWriter.String(),
				Error:   runtimeInfo.ErrorWriter.String(),
				Err:     errors.Wrap(err, runtimeInfo.ErrorWriter.String()),
			}
			executorRes.Success = initLog + executorRes.Success
			result <- executorRes
			return
		}
		return
	}()
	return result
}

func (k *K8sExecutor) log(ctx context.Context, runtimeInfo K8sRuntimeInfo) string {
	initContainerName := runtimeInfo.GetInitContainerName()
	if len(initContainerName) == 0 {
		return ""
	}
	result := ""
	for _, item := range initContainerName {
		if item == "" {
			continue
		}

		cli, _, err := k8s.GetInClusterClient()
		if k.config.Config.K8SExecutor.ClientConfig != "" {
			cli, _, err = k8s.GetOutClusterClient(k.config.Config.K8SExecutor.ClientConfig)
		}
		if err != nil {
			result += fmt.Sprintf("\n%s", err.Error())
			continue
		}

		podOperator := k8s.NewPod()
		log := podOperator.Log(ctx, cli, runtimeInfo.Namespace, runtimeInfo.PodName, item)
		if log != "" {
			result += fmt.Sprintf("\n初始化容器日志 %s:\n%s", item, log)
		}
	}
	return result
}

func (k *K8sExecutor) execute(ctx context.Context, param pipeline.ExecutorParam, runtimeInfo *K8sRuntimeInfo) error {
	//初始化链接
	cli, config, err := k8s.GetInClusterClient()
	if k.config.Config.K8SExecutor.ClientConfig != "" {
		cli, config, err = k8s.GetOutClusterClient(k.config.Config.K8SExecutor.ClientConfig)
	}
	if err != nil {
		return err
	}

	//创建工作目录
	mkdirShell := fmt.Sprintf("mkdir -p %s", k.shellWorkspace(param.GetStep()))
	util.SystemLog(ctx, "初始化k8s执行环境：pod %s 创建目录 %s", runtimeInfo.PodName, mkdirShell)
	util.SetFirstStageStepInitLog(k.p, "🚀在工作负载中创建目录：%s", mkdirShell)
	err = k.execShell(ctx, cli, config, param, runtimeInfo, mkdirShell)
	if err != nil {
		return err
	}

	//拷贝命令和文件
	files := runtimeInfo.Files
	filesToCopy := map[string]string{}
	for _, item := range files {
		filesToCopy[item.Path] = item.Content
	}
	filesToCopy[k.shellFileName(param.GetStep())] = param.GetCmd()
	util.SystemLog(ctx, "初始化k8s执行环境：pod %s 拷贝文件 %s", runtimeInfo.PodName, litter.Sdump(filesToCopy))
	util.SetFirstStageStepInitLog(k.p, "🚀初始化文件到工作负载: \n%s\n\t", litter.Sdump(funk.Keys(filesToCopy)))
	err = k.copyFile(ctx, cli, config, runtimeInfo, filesToCopy)
	if err != nil {
		util.SystemLog(ctx, "初始化k8s执行环境：pod %s 拷贝文件 失败: %s", runtimeInfo.PodName, err.Error())
		util.SetFirstStageStepInitLog(k.p, "🚀初始化文件失败： %s", err.Error())
		return err
	}
	//执行命令
	util.SystemLog(ctx, "初始化k8s执行环境：%s 执行主要命令 %s", runtimeInfo.PodName, litter.Sdump(runtimeInfo))
	util.SetFirstStageStepInitLog(k.p, "🚀============在工作负载中执行主要任务===========")
	err = k.runCmd(ctx, cli, config, param, runtimeInfo)
	if err != nil {
		return err
	}
	return nil
}

//从pod复制文件

func (k *K8sExecutor) copyFile(ctx context.Context, cli *kubernetes.Clientset, config *rest.Config, runtimeInfo *K8sRuntimeInfo, data map[string]string) error {
	cp := k8s.NewK8sCp()
	for key, item := range data {
		err := cp.CpString(ctx, cli, config, k8s.CpStringParam{
			Namespace:     runtimeInfo.Namespace,
			PodName:       runtimeInfo.PodName,
			ContainerName: runtimeInfo.ContainerName,
			Content:       item,
			Filename:      key,
		})
		if err != nil {
			return errors.Wrap(err, err.Error())
		}
	}
	return nil
}

func (k *K8sExecutor) execShell(ctx context.Context, cli *kubernetes.Clientset, config *rest.Config, param pipeline.ExecutorParam, runtimeInfo *K8sRuntimeInfo, shell string) error {
	exec := k8s.NewExec()
	err := exec.RunSh(ctx, cli, config, &k8s.ExecShellFileParam{
		Namespace:     runtimeInfo.Namespace,
		PodName:       runtimeInfo.PodName,
		ContainerName: runtimeInfo.ContainerName,
		Shell:         shell,
		Timeout:       param.GetTimeout(),
		SuccessWriter: runtimeInfo.SuccessWriter,
		ErrorWriter:   runtimeInfo.ErrorWriter,
	})
	if err != nil {
		return err
	}

	return nil
}

func (k *K8sExecutor) runCmd(ctx context.Context, cli *kubernetes.Clientset, config *rest.Config, param pipeline.ExecutorParam, runtimeInfo *K8sRuntimeInfo) error {
	//获取TTY配置通过环境变量
	exec := k8s.NewExec()
	err := exec.RunSh(ctx, cli, config, &k8s.ExecShellFileParam{
		Namespace:     runtimeInfo.Namespace,
		PodName:       runtimeInfo.PodName,
		ContainerName: runtimeInfo.ContainerName,
		Shell:         fmt.Sprintf("cd %s && %s", k.shellWorkspace(param.GetStep()), k.shellFileName(param.GetStep())),
		Timeout:       param.GetTimeout(),
		SuccessWriter: runtimeInfo.SuccessWriter,
		ErrorWriter:   runtimeInfo.ErrorWriter,
		HasTTy:        true,
	})
	if err != nil {
		if exec.IsTimeout {
			util.SetFirstStageStepInitLog(k.p, "🚀退出执行任务")
			return fmt.Errorf("超过最大执行时间，终止任务...\n%s", err.Error())
		}
		return err
	}
	return nil
}

func (k *K8sExecutor) getPodName() string {
	p := k.p
	//TODO 修改为整个pod的配置的hash,如果有变化说明不是同一个podp配置
	podSpac := v1.PodSpec{}
	_ = k.mPodSpecRecursion(&podSpac, p.GetStages())
	hash := util.HashPodObj(podSpac)
	return fmt.Sprintf("pipelinex-%s", hash)
}

func (k *K8sExecutor) getNamespace() string {
	//通过公共配置获取
	return "default"
}

func (k *K8sExecutor) isBeOfUsePod(pod *v1.Pod) bool {
	if pod == nil {
		return false
	}
	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed || pod.Status.Phase == v1.PodUnknown {
		return false
	}
	if pod.Status.Phase == v1.PodRunning && pod.ObjectMeta.DeletionTimestamp != nil {
		return false
	}
	return true
}

func (k *K8sExecutor) checkPodIsRunning(ctx context.Context, pod *k8s.Pod, cli *kubernetes.Clientset, p pipeline.PipelineOperator, podDescribe *v1.Pod) error {
	namespace, podname := k.getNamespace(), k.getPodName()
	podInfo, _ := pod.Info(ctx, cli, namespace, podname)
	if !k.isBeOfUsePod(podInfo) {
		//判断如果pod不是在运行中直接将pod删除
		if !k.isBeOfUsePod(podInfo) && podInfo != nil {
			err := pod.Delete(ctx, cli, namespace, podname)
			if err != nil {
				util.SystemLog(ctx, "创建工作pod前置删除pod操作失败：%s", err.Error())
				util.SetFirstStageStepInitLog(k.p, "🛸清理旧工作负载失败:%s", err.Error())
			}
		}
		for {
			//直到pod被删除才进行下一步重新创建
			podInfo, _ := pod.Info(ctx, cli, namespace, podname)
			if podInfo == nil {
				break
			}
			time.Sleep(time.Second)
		}
		//需要创建pod
		err := pod.Create(ctx, cli, namespace, podDescribe)
		if err != nil {
			return err
		}
	}
	for {
		//循环获取当前pod的状态直到pod已经准备就绪
		podInfo, err := pod.Info(ctx, cli, namespace, podname)
		if err != nil {
			util.SystemLog(ctx, "查找pod失败，pod没有被创建：%s", err.Error())
			util.SetFirstStageStepInitLog(k.p, "🛸工作负载创建失败: %s", err.Error())
			return err
		}
		status := podInfo.Status.Phase
		waitReason, isSuccess := pod.WaitReason(ctx, cli, namespace, podname)
		util.SystemLog(ctx, "流水线环境准备中%s", string(status))
		util.SetFirstStageStepInitLog(k.p, "🛸工作负载准备中: %s", string(status))
		p.Notify()
		if !isSuccess {
			err := errors.New(waitReason)
			util.SystemLog(ctx, err)
			return err
		}
		if status == v1.PodFailed {
			err := fmt.Errorf("pod create failed:%s log:%s", status, pod.GetFailPodLog(ctx, cli, namespace, podname))
			util.SetFirstStageStepInitLog(k.p, "🛸工作负载创建失败: %s \n🛸日志信息: \n%s", status, pod.GetFailPodLog(ctx, cli, namespace, podname))
			util.SystemLog(ctx, err)
			return err
		}
		util.SystemLog(ctx, "pod %s 等待理由：%s", podname, waitReason)
		util.SetFirstStageStepInitLog(k.p, "🛸工作负载准备完毕正在初始化：%s", waitReason)
		if status == v1.PodPending || !k.isBeOfUsePod(podInfo) {
			time.Sleep(time.Second * 4)
			continue
		}
		if status == v1.PodRunning {
			if !k.isBeOfUsePod(podInfo) {
				time.Sleep(time.Second * 4)
				continue
			}
			//如果pod在运行中了，但是容器还未准备好就抛出错误
			if err, needExit := pod.ContainerIsReady(ctx, cli, namespace, podname); err != nil {
				util.SystemLog(ctx, "容器还未准备好：%s", err.Error())
				util.SetFirstStageStepInitLog(k.p, "🛸工作负载状态异常：%s", err.Error())
				if needExit {
					return err
				}
				continue
			}
			break
		}
	}
	return nil
}

func (k *K8sExecutor) shellFileName(step pipeline.Step) string {
	return fmt.Sprintf("%s%c%s", k.shellWorkspace(step), util.PathSeparator, "boot.sh")
}

func (k *K8sExecutor) shellWorkspace(step pipeline.Step) string {
	return util.GetStepWorkdir(step)
}
