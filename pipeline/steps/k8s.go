package steps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/executor"
	"github.com/chenyingqiao/pipeline-lib/pipeline/templete"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	jsoniter "github.com/json-iterator/go"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
	v1 "k8s.io/api/core/v1"
)

// K8sCustomStageType yaml中定义的类型
const K8sCustomStageType = "k8s"

// K8sStep step
type K8sStep struct {
	BaseStep
	cmds        []string
	k8sExecutor pipeline.Executor
	config      K8sStepParam
}

// K8sStepParam 入参格式
type K8sStepParam struct {
	pipeline.BaseParam `mapstructure:",squash"`
	Shell              []string            `mapstructure:"shell"`
	Files              []executor.K8sFiles `mapstructure:"files"`
	Envs               []Kv                `mapstructure:"envs"`
	BeforeAction       []ContainerOption   `mapstructure:"beforeAction"`
}

// ContainerOption 容器配置
type ContainerOption struct {
	Image string `mapstructure:"image"`
	Cmd   string `mapstructure:"cmd"`
}

// NewK8sStep 实例化shell
func NewK8sStep(p pipeline.PipelineOperator, stage pipeline.StageOperator, data interface{}) (pipeline.StepOperator, error) {
	if _, ok := data.(map[string]interface{}); !ok {
		return nil, errors.New("data type error: need map[string]string")
	}
	dataMap := data.(map[string]interface{})
	if !util.IsExist(dataMap, "type") || util.GetV(dataMap, "type", "").(string) != K8sCustomStageType {
		return nil, pipeline.ErrStepTypeNotMatch
	}
	if err := util.Check(dataMap, []string{"image", "name", "shell", "type"}); err != nil {
		return nil, err
	}

	dataStruct := K8sStepParam{}
	err := mapstructure.Decode(dataMap, &dataStruct)
	if err != nil {
		return nil, err
	}

	//附加code到metadata中，用于让用户可以通过占位符直接获取数据数据
	dataStruct.Code = cast.ToString(stage.GetConfig()["code"])
	if dataStruct.Code != "" {
		dataStruct.Metadata = append(dataStruct.Metadata, pipeline.Metadata{
			Key:   dataStruct.Code,
			Value: "{{ getOutput }}",
		})
	}

	for _, item := range dataStruct.BeforeAction {
		if item.Cmd == "" || item.Image == "" {
			return nil, fmt.Errorf("beforeAction: options [image name cmd] is required")
		}
	}

	//生成steps
	step := K8sStep{
		BaseStep:    NewBaseStep(p, stage, dataStruct.ID),
		cmds:        dataStruct.Shell,
		config:      dataStruct,
		k8sExecutor: executor.NewK8sExecutor(p),
	}
	return &step, nil
}

// Exec 执行当前step
func (kss *K8sStep) Exec(ctx context.Context) error {
	if err := kss.BaseStep.Exec(ctx); err != nil {
		return err
	}
	//解析转换参数中的模板
	kss.config = ParseObject(ctx, kss.pipeline, kss.stage, kss, kss.config)
	//刷新环境变量并执行主容器代码
	cmdStr := strings.Join(kss.cmds, ";\n")
	err := kss.exec(ctx, cmdStr)
	if err != nil {
		kss.err = err
		return err
	}
	return nil
}

// Env 获取运行环境配置信息
func (kss *K8sStep) Env() (interface{}, pipeline.EnvType) {
	//解析转换参数中的模板
	kss.BaseStep.initPublicVars()
	kss.config = ParseObject(context.Background(), kss.pipeline, kss.stage, kss, kss.config)
	// 获取当前时间
	now := time.Now()
	// 构造明天凌晨4点的时间
	tomorrow := now.Add(24 * time.Hour) // 加一天，即明天
	targetTime := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 4, 0, 0, 0, tomorrow.Location())
	// 计算时间差
	duration := targetTime.Sub(now)
	// 获取时间差的秒数
	sleepTime := cast.ToString(int(duration.Seconds()))
	containers := []v1.Container{
		{
			Name:  util.GetStepContainerName(kss),
			Image: kss.config.Image,
			Command: []string{
				"sleep",
				sleepTime,
			},
			Env: KvToK8sEnvVars(kss.config.Envs),
		},
	}
	for key, item := range kss.config.BeforeAction {
		containers = append(containers, v1.Container{
			Name:  fmt.Sprintf("%s-before-%d", util.GetStepContainerName(kss), key),
			Image: item.Image,
			Command: []string{
				"sleep",
				sleepTime,
			},
			Env: KvToK8sEnvVars(kss.config.Envs),
		})
	}
	return v1.PodSpec{
		Containers: containers,
	}, pipeline.K8s
}

// IsTermination 是否结束后面的流水线
func (kss *K8sStep) IsTermination() bool {
	return kss.isTermination(kss.config.Termination)
}

func (kss *K8sStep) exec(ctx context.Context, cmd string) error {
	defer kss.pipeline.Notify()
	//stage配置信息
	runtimeInfo := executor.NewK8sRuntimeInfo(kss.pipeline, kss, kss.config.Files)
	rchan := kss.k8sExecutor.Do(ctx, executor.ExecutorParam{
		CMD:         cmd,
		Timeout:     kss.timeout,
		RuntimeInfo: runtimeInfo,
		Step:        kss,
	})
	defer func() {
		//设置metadata
		for _, item := range kss.config.Metadata {
			valueParsed := templete.Parse(
				ctx,
				item.Value,
				kss.pipeline.GetMetadata(),
				false,
				templete.GetDefaultParseFnMap(ctx, kss.pipeline, kss.stage, kss, runtimeInfo, kss.pipeline.GetMetadata()),
			)
			if jsoniter.Valid([]byte(valueParsed)) {
				var vObj interface{}
				jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal([]byte(valueParsed), &vObj)
				kss.pipeline.SetMetadata(item.Key, vObj)
				continue
			}
			kss.pipeline.SetMetadata(
				item.Key,
				valueParsed,
			)
		}
	}()
	var hasErr error
	defer func() {
		if hasErr != nil {
			kss.log += "\n❌执行失败"
			return
		}
		kss.log += "\n✅执行成功"
	}()
	for {
		select {
		case result, ok := <-rchan:
			if !ok {
				util.SystemLog(ctx, "日志信息: %s\n", kss.log)
				return nil
			}
			var err error
			kss.log, err = kss.GetLogFronExecutorResult(result)
			if util.IsTerminatorErr(err) {
				hasErr = err
				return pipeline.ErrTimeoutOrCancel
			}
			if err != nil {
				hasErr = err
				return err
			}
		}
	}
}

// GetBaseParam 获取基础配置
func (kss *K8sStep) GetBaseParam() pipeline.BaseParam {
	return kss.config.BaseParam
}
