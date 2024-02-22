package pipeline

import "github.com/pkg/errors"

var (
	//ErrTimeoutOrCancel 流水线超时
	ErrTimeoutOrCancel = errors.New("timeout or cancel")

	//ErrCast 类型转换错误
	ErrCast = errors.New("type is error")

	//ErrStepTypeNotMatch Step type is not match
	ErrStepTypeNotMatch = errors.New("step type is not match or type is empty")
	ErrStageTypeNotMatch = errors.New("stage type is not match or type is empty")

	//ErrStepRetryMaxTime 超过最大重试次数
	ErrStepRetryMaxTime = errors.New("retry max time")

	ErrRemoteExecuteFail = errors.New("remote execute failed")
)
