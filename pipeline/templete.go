package pipeline

type Templete interface {
	//通过模板解析出对应字字符
	Parse(temp string, data map[string]interface{}) string
}
