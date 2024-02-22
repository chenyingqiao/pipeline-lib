package util

import (
	"encoding/json"
	"strings"

	"golang.org/x/exp/constraints"
)

// GetV key存在就获取key,不存在就使用默认值
func GetV[K constraints.Ordered, V any](data map[K]V, key K, defaultV V) V {
	if _, ok := data[key]; !ok {
		return defaultV
	}
	return data[key]
}

// IsExist 是否存在
func IsExist[K constraints.Ordered, V any](data map[K]V, key ...K) bool {
	for _, item := range key {
		if _, ok := data[item]; !ok {
			return false
		}
	}
	return true
}

// Merge 合并map
func Merge[K constraints.Ordered, V any](m1 map[K]V, m ...map[K]V) map[K]V {
	result := map[K]V{}
	for k, item := range m1 {
		result[k] = item
	}
	for _, item := range m {
		for k, v := range item {
			result[k] = v
		}
	}
	return result
}

// MergeRecursiveJson 递归合并json
func MergeRecursiveJson(m string, m2 string) (string, error) {
	mMap := map[string]interface{}{}
	m2Map := map[string]interface{}{}
	err := json.Unmarshal([]byte(m), &mMap)
	if err != nil {
		return m, err
	}
	err = json.Unmarshal([]byte(m2), &m2Map)
	if err != nil {
		return m, err
	}
	rMap := MergeRecursive(mMap, m2Map)
	j, err := json.Marshal(rMap)
	if err != nil {
		return m, err
	}
	return string(j), nil
}

// MergeRecursive 递归合并
func MergeRecursive(m map[string]interface{}, m2 ...map[string]interface{}) map[string]interface{} {
	result := m
	for _, item := range m2 {
		for k, v := range item {
			_, vIsMap := v.(map[string]interface{})
			_, rIsExists := result[k]
			var rIsMap bool
			if rIsExists {
				_, rIsMap = result[k].(map[string]interface{})
			}
			if rIsMap && vIsMap {
				//递归合并map
				result[k] = MergeRecursive(result[k].(map[string]interface{}), v.(map[string]interface{}))
				continue
			}
			result[k] = v
		}
	}
	return result
}

// QueryMap 多层级map
func QueryMap(data interface{}, query string, defaultV interface{}) interface{} {
	indexSplit := strings.Split(query, ".")
	for _, v := range indexSplit {
		if v == "" {
			continue
		}
		if _, ok := data.(map[string]interface{})[v]; !ok {
			return defaultV
		}
		data = data.(map[string]interface{})[v]
	}
	return data
}
