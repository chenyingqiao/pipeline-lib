package util

import (
	"crypto/md5"
	"encoding/hex"

	v1 "k8s.io/api/core/v1"
)

func HashPodObj(obj v1.PodSpec) string {
	content := ""
	for _, item := range obj.InitContainers {
		content += item.Image
		for _, eitem := range item.Env {
			content += eitem.Name + eitem.Value
		}
	}
	for _, item := range obj.Containers {
		content += item.Image
		for _, eitem := range item.Env {
			content += eitem.Name + eitem.Value
		}
	}
	hash := md5.Sum([]byte(content))
	hashStr := hex.EncodeToString(hash[:])
	return hashStr
}
