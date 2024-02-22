package templete

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"text/template"

	"github.com/antlabs/pcurl"

	jsoniter "github.com/json-iterator/go"
	"github.com/oliveagle/jsonpath"
	"github.com/pkg/errors"

	"github.com/chenyingqiao/pipeline-lib/pipeline"
	"github.com/chenyingqiao/pipeline-lib/pipeline/util"
	"github.com/Masterminds/sprig/v3"
	"gopkg.in/yaml.v3"
)

func HasReplaceFlag(temp string) bool {
// 正则表达式模式
	pattern := `\{\{.+\}\}`

	// 使用正则表达式进行匹配
	match, _ := regexp.MatchString(pattern, temp)	
	return match
}

func ParseObject[T any](ctx context.Context, temp T, data map[string]interface{}, fnMap template.FuncMap) T {
	//前置判断和是否调用parseObjCall的标记
	if data == nil {
		data = map[string]interface{}{}
	}
	//转换temp为map[string]interface{}
	var tempData interface{}
	byteStr, err := jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(temp)
	if err != nil {
		return temp
	}
	if !HasReplaceFlag(string(byteStr)) {
		//如果不需要替换就
		return temp
	}
	err = jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal(byteStr, &tempData)
	if err != nil {
		return temp
	}

	//递归的进行数据处理
	if tmepDataMap, ok := tempData.(map[string]interface{}); ok {
		for key, item := range tmepDataMap {
			switch item.(type) {
			case string:
				tmepDataMap[key] = Parse(ctx, item.(string), data, true, fnMap)
			case interface{}:
				tmepDataMap[key] = ParseObject(ctx, item, data, fnMap)
			case []interface{}:
				itemData := item.([]interface{})
				for arrIndex, arrItem := range itemData {
					if _, ok := arrItem.(string); ok {
						itemData[arrIndex] = Parse(ctx, arrItem.(string), data, true, fnMap)
						continue
					}
					itemData[arrIndex] = ParseObject(ctx, arrItem, data, fnMap)
				}
				tmepDataMap[key] = itemData
			case []string:
				itemData := item.([]string)
				for arrIndex, arrItem := range itemData {
					itemData[arrIndex] = Parse(ctx, arrItem, data, true, fnMap)
				}
				tmepDataMap[key] = itemData
			case map[string]interface{}:
				itemData := item.(map[string]interface{})
				for mapIndex, mapItem := range itemData {
					if _, ok := mapItem.(string); ok {
						itemData[mapIndex] = Parse(ctx, mapItem.(string), data, true, fnMap)
						continue
					}
					itemData[mapIndex] = ParseObject(ctx, mapItem, data, fnMap)
				}
				tmepDataMap[key] = itemData
			}
		}
	}
	if tmepDataArr, ok := tempData.([]interface{}); ok {
		for key, item := range tmepDataArr {
			switch item.(type) {
			case string:
				tmepDataArr[key] = Parse(ctx, item.(string), data, true, fnMap)
			case interface{}:
				tmepDataArr[key] = ParseObject(ctx, item, data, fnMap)
			case []interface{}:
				itemData := item.([]interface{})
				for arrIndex, arrItem := range itemData {
					if _, ok := arrItem.(string); ok {
						itemData[arrIndex] = Parse(ctx, arrItem.(string), data, true, fnMap)
						continue
					}
					itemData[arrIndex] = ParseObject(ctx, arrItem, data, fnMap)
				}
				tmepDataArr[key] = itemData
			case []string:
				itemData := item.([]string)
				for arrIndex, arrItem := range itemData {
					itemData[arrIndex] = Parse(ctx, arrItem, data, true, fnMap)
				}
				tmepDataArr[key] = itemData
			case map[string]interface{}:
				itemData := item.(map[string]interface{})
				for mapIndex, mapItem := range itemData {
					if _, ok := mapItem.(string); ok {
						itemData[mapIndex] = Parse(ctx, mapItem.(string), data, true, fnMap)
						continue
					}
					itemData[mapIndex] = ParseObject(ctx, mapItem, data, fnMap)
				}
				tmepDataArr[key] = itemData
			}
		}
	}

	//将数据转换后注入到temp
	byteStr, err = jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(tempData)
	if err != nil {
		return temp
	}
	err = jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal(byteStr, &temp)
	if err != nil {
		return temp
	}

	return temp
}

func GetDefaultParseFnMap(ctx context.Context, p pipeline.Pipeline, stage pipeline.Stage, step pipeline.Step, runtimeInfo interface{}, data map[string]interface{}) template.FuncMap {
	return template.FuncMap{
		"getFileContent": func(path string) string {
			fns := []GetFileContentFunc{
				K8sGetFileContent,
				LocalGetFileContent,
			}
			var content string
			for _, item := range fns {
				if stage == nil || step == nil || runtimeInfo == nil {
					continue
				}
				contentBak, err := item(ctx, p, stage, step, runtimeInfo, path)
				if err != nil && !errors.Is(err, RuntimeInfoErr) {
					return err.Error()
				}
				if err != nil {
					continue
				}
				content = contentBak
			}
			return content
		},
		"getMetadata": func(k string) interface{} {
			if _, ok := data[k]; !ok {
				return ""
			}
			return data[k]
		},
		"getData": func() (interface{}, error) {
			if stage == nil || step == nil || runtimeInfo == nil {
				return "", errors.New("流水线运行时和节点数据为空")
			}
			data, err := util.ParseOutPutData(step.GetLog())
			if err != nil {
				return "", errors.New("数据解析失败")
			}
			return data, nil
		},
		"getOutput": func() (interface{}, error) {
			if stage == nil || step == nil || runtimeInfo == nil {
				return "", errors.New("流水线运行时和节点数据为空")
			}
			data, err := util.ParseOutPutData(step.GetLog())
			if err != nil {
				return "", errors.New("数据解析失败")
			}
			return data, nil
		},
		"jsonPath": func(path string) (interface{}, error) {
			data, err := util.ParseOutPutData(step.GetLog())
			if err != nil {
				return "", errors.New("数据解析失败")
			}
			return jsonpath.JsonPathLookup(data, path)
		},
	}
}

func isYaml(temp string) interface{} {
	_, err := pcurl.ParseAndRequest(temp)
	if err == nil && strings.Contains(temp, "curl") {
		return nil
	}
	if strings.HasPrefix(strings.Trim(temp, "\n \t"), "{") {
		return nil
	}
	//多阶段的yaml就不进行解析了直接跳过
	if strings.Contains(temp, "\n---\n") {
		return nil
	}
	result := map[string]interface{}{}
	err = yaml.Unmarshal([]byte(temp), &result)
	if err != nil {
		return nil
	}
	return result
}

func Parse(ctx context.Context, tempIface interface{}, data map[string]interface{}, parseStrToObj bool, tf template.FuncMap) string {
	var temp string
	if _, ok := tempIface.(string); !ok {
		return ""
	}
	temp = tempIface.(string)
	if temp == "" {
		return ""
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	//判断是否是yaml,如果是的话就调用obj解析进行解析
	if obj := isYaml(temp); obj != nil && parseStrToObj {
		obj = ParseObject(ctx, obj, data, tf)
		result, err := yaml.Marshal(obj)
		if err != nil {
			return temp
		}
		return string(result)
	}

	//添加额外的函数
	templateEngine, err := template.New("temp").Option("missingkey=error").Funcs(sprig.TxtFuncMap()).Funcs(tf).Parse(temp)
	if err != nil {
		return temp
	}
	var byteBuf bytes.Buffer
	err = templateEngine.Execute(&byteBuf, data)
	if err != nil {
		return temp
	}
	resultStr := byteBuf.String()
	return resultStr
}
