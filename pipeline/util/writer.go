package util

import (
	"bytes"
	"sync"
)

//BufferWriterSync 线程安全的bufferwriter
type BufferWriterSync struct {
	b bytes.Buffer
	l sync.RWMutex
}

func NewBufferWriterSync() *BufferWriterSync {
	return &BufferWriterSync{
		b: bytes.Buffer{},
		l: sync.RWMutex{},
	}
}

//Write 读取字节
func (bws *BufferWriterSync) Write(p []byte) (n int, err error) {
	bws.l.Lock()
	defer bws.l.Unlock()
	//写入数据到b
	return bws.b.WriteString(string(p))
}

//String 转成string
func (bws *BufferWriterSync) String() string {
	bws.l.RLock()
	defer bws.l.RUnlock()
	return bws.b.String()
}

//Len 缓存长度
func (bws *BufferWriterSync) Len() int {
	bws.l.RLock()
	defer bws.l.RUnlock()
	return bws.b.Len()
}
