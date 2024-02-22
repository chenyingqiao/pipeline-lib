package util

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
)

// GetStepContainerName 获取容器名称
func GetStepContainerName(step pipeline.StepOperator) string {
	refType := reflect.TypeOf(step)
	name := strings.ToLower(refType.Elem().Name())
	return fmt.Sprintf("%s-%s", name, Md5(step.GetBaseParam().Image))
}
