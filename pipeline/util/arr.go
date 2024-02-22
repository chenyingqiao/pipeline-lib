package util

import (
	"golang.org/x/exp/constraints"
	v1 "k8s.io/api/core/v1"
)

// Compared 可比较的
type Compared interface {
	v1.PodPhase | constraints.Ordered
}

// InArray 是否在数组中
func InArray[T Compared](a T, b []T) bool {
	for _, item := range b {
		if a == item {
			return true
		}
	}
	return false
}

// LastItem 获取数组的最后一个元素
func LastItem[T any](a []T) T {
	return a[len(a)-1]
}

