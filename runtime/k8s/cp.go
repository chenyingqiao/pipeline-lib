package k8s

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

//Cp 拷贝文件
type Cp struct{}

func NewK8sCp() *Cp {
	return &Cp{}
}

const windowspath = "D:\\tmp"

//CpStringParam 拷贝参数
type CpStringParam struct {
	Namespace     string
	PodName       string
	ContainerName string
	Content       string
	Filename      string
}

//CpString 通过启动的sidecar进行文件内容传递 如果没有启动sidecar容器就使用普通的apiserver进行文件传输
func (c *Cp) CpString(ctx context.Context, cli *kubernetes.Clientset, config *rest.Config, param CpStringParam) error {
	//发送请求生成文件
	pod, err := NewPod().Info(ctx, cli, param.Namespace, param.PodName)
	if err != nil {
		util.SystemLog(ctx, "pod:%s error:%s\n", param.PodName, err.Error())
		return err
	}
	podIP := pod.Status.PodIP
	podURL := fmt.Sprintf("http://%s:9999/text-store", podIP)
	util.SystemLog(ctx, "pod:%s url:%s\n", param.PodName, podURL)
	resp, err := resty.New().SetTimeout(time.Second * 5).R().SetBody([]map[string]string{
		{
			"path":    param.Filename,
			"content": param.Content,
		},
	}).Post(podURL)
	if err != nil {
		util.SystemLog(ctx,"pod:%s url:%s %s\n", param.PodName, podURL, err.Error())
		//如果不存在容器的话使用apiserver进行拷贝
		return c.cpStringByApiServer(ctx, cli, config, param)
	}
	if resp.IsError() {
		util.SystemLog(ctx,"file send to pod by sidecar is fail, status code %d\n", resp.StatusCode())
		return errors.New("file send to pod by sidecar is fail")
	}
	return nil
}

//CpString 拷贝字符串到对应的pod中
func (c *Cp) cpStringByApiServer(ctx context.Context, cli *kubernetes.Clientset, config *rest.Config, param CpStringParam) error {
	fileName := filepath.Base(param.Filename)
	localPathStr := fmt.Sprintf("/tmp/%s", fileName)
	destDir := filepath.Dir(param.Filename)
	//生成文本到文件
	if runtime.GOOS == "windows" {
		localPathStr = fmt.Sprintf("%s\\%s", windowspath, fileName)
		destDir = strings.ReplaceAll(destDir, "\\", "/")
	}
	defer os.Remove(localPathStr)
	err := ioutil.WriteFile(localPathStr, []byte(param.Content), 0777)
	if err != nil {
		return errors.Wrap(err, err.Error())
	}
	//拷贝文件到pod的容器中
	reader, writer := io.Pipe()
	stdOut := &bytes.Buffer{}
	stdErr := &bytes.Buffer{}
	srcFile := newLocalPath(localPathStr)
	destFile := newRemotePath(param.Filename)
	go func(src localPath, dest remotePath, writer io.WriteCloser) {
		defer writer.Close()
		makeTar(src, dest, writer)
	}(srcFile, destFile, writer)
	cmdArr := []string{"tar", "-xmf", "-", "-C", destDir}
	req := cli.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(param.PodName).
		Namespace(param.Namespace).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Container: param.ContainerName,
			Command:   cmdArr,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return errors.Wrap(err, err.Error())
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  reader,
		Stdout: stdOut,
		Stderr: stdErr,
		Tty:    false,
	})
	if err != nil {
		return errors.Wrap(err, err.Error())
	}
	if runtime.GOOS == "windows" {
		successBuffer := bytes.Buffer{}
		errorBuffer := bytes.Buffer{}
		err = NewExec().RunSh(ctx, cli, config, &ExecShellFileParam{
			Namespace:     param.Namespace,
			PodName:       param.PodName,
			ContainerName: param.ContainerName,
			Shell:         fmt.Sprintf("chmod 777 %s/%s", destDir, fileName),
			SuccessWriter: &successBuffer,
			ErrorWriter:   &errorBuffer,
		})
		if err != nil {
			return errors.Wrap(err, err.Error())
		}
	}
	return nil
}

func makeTar(src localPath, dest remotePath, writer io.Writer) error {
	tarWriter := tar.NewWriter(writer)
	defer tarWriter.Close()

	srcPath := src.Clean()
	destPath := dest.Clean()
	return recursiveTar(srcPath.Dir(), srcPath.Base(), destPath.Dir(), destPath.Base(), tarWriter)
}

func recursiveTar(srcDir, srcFile localPath, destDir, destFile remotePath, tw *tar.Writer) error {
	matchedPaths, err := srcDir.Join(srcFile).Glob()
	if err != nil {
		return err
	}
	for _, fpath := range matchedPaths {
		stat, err := os.Lstat(fpath)
		if err != nil {
			return err
		}
		if stat.IsDir() {
			files, err := ioutil.ReadDir(fpath)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				//case empty directory
				hdr, _ := tar.FileInfoHeader(stat, fpath)
				hdr.Name = destFile.String()
				if err := tw.WriteHeader(hdr); err != nil {
					return err
				}
			}
			for _, f := range files {
				if err := recursiveTar(srcDir, srcFile.Join(newLocalPath(f.Name())),
					destDir, destFile.Join(newRemotePath(f.Name())), tw); err != nil {
					return err
				}
			}
			return nil
		} else if stat.Mode()&os.ModeSymlink != 0 {
			//case soft link
			hdr, _ := tar.FileInfoHeader(stat, fpath)
			target, err := os.Readlink(fpath)
			if err != nil {
				return err
			}

			hdr.Linkname = target
			hdr.Name = destFile.String()
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
		} else {
			//case regular file or other file type like pipe
			hdr, err := tar.FileInfoHeader(stat, fpath)
			if err != nil {
				return err
			}
			hdr.Name = destFile.String()

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			f, err := os.Open(fpath)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
			return f.Close()
		}
	}
	return nil
}
