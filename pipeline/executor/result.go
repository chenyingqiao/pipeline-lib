package executor

import "github.com/chenyingqiao/pipeline-lib/pipeline"

//Result 执行返回
type Result struct {
	Success string
	Error   string
	Err     error
}

//GetLog 取得日志信息
func (kr Result) GetLog() pipeline.ExecutorLog {
	return pipeline.ExecutorLog{
		Success: kr.Success,
		Error:   kr.Error,
	}
}

//GetErr 获取程序错误信息
func (kr Result) GetErr() error {
	return kr.Err
}
