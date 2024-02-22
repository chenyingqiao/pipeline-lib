package util

import (
	"context"
	"strings"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/pkg/errors"
)

func IsTerminatorErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, pipeline.ErrTimeoutOrCancel) {
		return true
	}
	if strings.Contains(err.Error(), context.Canceled.Error()) || 
		strings.Contains(err.Error(), context.DeadlineExceeded.Error()) || 
		strings.Contains(err.Error(), pipeline.ErrTimeoutOrCancel.Error()){
		return true
	}
	return false
}
