package util

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/golang/glog"
	"github.com/sanity-io/litter"
)

const startSpan = "@==========Start Result Span==========@"
const endSpan = "@==========End Result Span==========@"
const UpdateState = "UpdateState"

// InterceptString 截取字符串
func InterceptString(str string, strLen int64) (string, string) {
	strLen = strLen / 2
	runStr := []rune(str)
	nRuneLen := len(runStr)
	str1 := string(runStr[0:strLen])
	str2 := string(runStr[strLen:nRuneLen])
	return str1, str2
}

// ParseOutPutData 解析文件中返回的数据
func ParseOutPutData(text string) (string, error) {
	// 找到开始和结束的位置
	startIndex := strings.Index(text, startSpan) + len(startSpan)
	endIndex := strings.Index(text, endSpan)

	// 截取字符串
	substring := text[startIndex:endIndex]
	return substring, nil
}

func GetStackTrace() []string {
	pc := []uintptr{} // 设置一个足够大的数组来保存调用堆栈帧
	n := runtime.Callers(0, pc)
	frames := runtime.CallersFrames(pc[:n])

	result := []string{}
	for frame, more := frames.Next(); more; frame, more = frames.Next() {
		result = append(result, fmt.Sprintf("%s:%d %s", frame.Function, frame.Line, frame.Function))
	}
	return result
}

// 打印整个流水线的运行日志
func SystemLog(ctx context.Context, msg interface{}, v ...interface{}) {
	var id, pid, status string = "", "", ""
	var stages interface{}
	p := pipeline.GetPipelineImplFromContext(ctx)
	if p != nil {
		id = p.GetID()
		status = ""
		stages = ""
		pid = p.GetParentID()
		if pid == "" {
			pid = p.GetID()
		}
	}
	stageInfoMap := map[string]interface{}{}
	if p != nil {
		stageInfo := p.GetStages()
		for _, item := range stageInfo {
			steps := map[string]string{}
			for _, sitem := range item.GetSteps() {
				steps["id"] = sitem.GetID()
				steps["status"] = sitem.GetStatus()
			}
			stageInfoMap[item.GetID()] = steps
		}
	}
	errMsg := msg
	if _, ok := msg.(error); ok {
		errMsg = msg.(error).Error()
	}
	glog.Errorf(`pipeline is %s.
		pipeline father id is %s. 
			pipeline status is %s. 
			stages status is %s. 
		error: %s`, id, pid, status, litter.Sdump(stages), errMsg)
}
