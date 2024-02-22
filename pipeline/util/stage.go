package util

import (
	"fmt"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/pkg/errors"
)

// Check 检查字段是否匹配
func Check(data map[string]interface{}, mustProperty []string) error {
	if !IsExist(data, mustProperty...) {
		return errors.New(fmt.Errorf("property is not match: %v is required", mustProperty).Error())
	}
	return nil
}

// 解析成stage列表
func ParseConfigToStages(pipe pipeline.PipelineOperator, stages []interface{}, fn ...pipeline.NewStage) ([]pipeline.StageOperator, error) {
	result := []pipeline.StageOperator{}
	for _, item := range stages {
		//替换公共模板
		for _, newStageFn := range fn {
			stageItem, err := newStageFn(pipe, item)
			if err != nil && errors.Is(err, pipeline.ErrStageTypeNotMatch) {
				continue
			}
			if err != nil {
				return nil, errors.WithMessage(err, "初始化stage列表错误")
			}
			result = append(result, stageItem)
		}
	}
	return result, nil
}
