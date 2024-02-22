package util

import (
	"io"
	"io/ioutil"
)

func Reader(r io.ReadCloser) string {
	str, _ := ioutil.ReadAll(r)
	return string(str)
}
