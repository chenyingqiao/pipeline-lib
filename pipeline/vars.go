package pipeline

import (
	"context"

)

var VERSION = "v0.2.0"
var ContainerSleepSecend = "3600"
type PipelineImplCtxKey struct{}


func ContextWithPipelineImpl(ctx context.Context,pipe PipelineOperator) context.Context {
	return context.WithValue(ctx,PipelineImplCtxKey{},pipe)
} 

func GetPipelineImplFromContext(ctx context.Context) PipelineOperator {
	p,ok := ctx.Value(PipelineImplCtxKey{}).(PipelineOperator)
	if ok {
		return p
	}
	return nil
}
