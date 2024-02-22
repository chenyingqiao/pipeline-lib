package util

import (
	"fmt"
	"os"
	"runtime"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
)

const PathSeparator = '/'

//FormatMetadata 格式化metadata
func FormatMetadata(data map[string]interface{}) string {
	result := ""
	for k, v := range data {
		result += fmt.Sprintf("%s ==> %s\n\t", k, v)
	}
	return result
}

//GetPublicWorkDirWithSys 获取公共工作目录
func GetPublicWorkDirWithSys(pipelineID string) string {
	tmpPath := fmt.Sprintf("%c%s", PathSeparator, "tmp")
	if runtime.GOOS == "windows" {
		pwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		tmpPath = pwd
	}
	err := os.MkdirAll(tmpPath, 0777)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s%c%s", tmpPath, PathSeparator, pipelineID)
}

//GetStepWorkdirWithSys 获取节点工作目录
func GetStepWorkdirWithSys(step pipeline.Step) string {
	tmpPath := fmt.Sprintf("%c%s", PathSeparator, "tmp")
	if runtime.GOOS == "windows" {
		pwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		tmpPath = pwd
	}
	err := os.MkdirAll(tmpPath, 0777)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s%c%s%c%s%c%s", tmpPath, PathSeparator, step.GetPipelineID(), PathSeparator, step.GetStageID(), PathSeparator, step.GetID())
}

//GetPublicWorkDir 获取公共工作目录
func GetPublicWorkDir() string {
	tmpPath := fmt.Sprintf("%c%s", PathSeparator, "tmp")
	return tmpPath
}

//GetStepWorkdir 获取节点工作目录
func GetStepWorkdir(step pipeline.Step) string {
	tmpPath := fmt.Sprintf("%c%s", PathSeparator, "tmp")
	return fmt.Sprintf("%s%c%s%c%s%c%s", tmpPath, PathSeparator, step.GetPipelineID(), PathSeparator, step.GetStageID(), PathSeparator, step.GetID())
}

//GetStepConfigPath 获取配置文件地址
func GetStepConfigPath(step pipeline.Step) string {
	return fmt.Sprintf("%s%c%s", GetStepWorkdir(step), PathSeparator, ".config.yaml")
}
