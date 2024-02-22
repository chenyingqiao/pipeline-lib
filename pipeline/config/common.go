package config

import (
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

type Pipeline struct {
	ID       string        `yaml:"ID"`
	PID      string        `yaml:"PID"`
	Config   Config        `yaml:"Config"`
	Params   Params        `yaml:"Params"`
	Stages   []interface{} `yaml:"Stages"`
	Metadata []Metadata    `yaml:"Metadata" json:"Metadata,omitempty"`
}
type K8SExecutor struct {
	SidecarImage          string `yaml:"SidecarImage" json:"SidecarImage,omitempty"`
	IsKeepPod             bool   `yaml:"IsKeepPod" json:"IsKeepPod,omitempty"`
	Destruction           bool   `yaml:"Destruction" json:"Destruction,omitempty"`
	Namespace             string `yaml:"Namespace" json:"Namespace,omitempty"`
	NetworkMode           string `yaml:"NetworkMode" json:"NetworkMode,omitempty"`
	ClientConfig          string `yaml:"ClientConfig" json:"ClientConfig,omitempty"`
	PodsNodeSelectorKey   string `yaml:"PodsNodeSelectorKey" json:"PodsNodeSelectorKey,omitempty"`
	PodsNodeSelectorValue string `yaml:"PodsNodeSelectorValue" json:"PodsNodeSelectorValue,omitempty"`
	LiveTime              int    `yaml:"LiveTime" json:"LiveTime,omitempty"`
}

// Metadata 元数据
type Metadata struct {
	Key   string      `yaml:"Key" json:"Key,omitempty"`
	Value interface{} `yaml:"Value" json:"Value,omitempty"`
}
type Config struct {
	K8SExecutor       K8SExecutor `yaml:"K8sExecutor" json:"K8sExecutor,omitempty"`
	AgentStage        AgentStage  `yaml:"AgentStage" json:"AgentStage,omitempty"`
	LogServerEndpoint string      `yaml:"LogServerEndpoint" json:"LogServerEndpoint,omitempty"`
	HubServerEndpoint string      `yaml:"HubServerEndpoint" json:"HubServerEndpoint,omitempty"`
	Timeout           int64       `yaml:"Timeout" json:"Timeout,omitempty"`
}
type AgentStage struct {
	Token       string `yaml:"Token" json:"Token,omitempty"`
	Tag         string `yaml:"Tag" json:"Tag,omitempty"`
	ExecURL     string `yaml:"ExecURL" json:"ExecURL,omitempty"`
	StateURL    string `yaml:"StateURL" json:"StateURL,omitempty"`
	TerminalURL string `yaml:"TerminalURL" json:"TerminalURL,omitempty"`
	MetadataURL string `yaml:"MetadataURL" json:"MetadataURL,omitempty"`
}
type Data map[string]interface{}
type Params struct {
	Data Data `yaml:"Data" json:"Data,omitempty"`
}

func NewPipelineConfig(config string) (Pipeline, error) {
	pConfig := Pipeline{}
	err := yaml.Unmarshal([]byte(config), &pConfig)
	if err != nil {
		return pConfig, errors.Wrap(err, "配置文件格式错误，请检查配置文件")
	}
	return pConfig, nil
}

func (p Pipeline) HasBeforeAction() bool {
	configBytes, err := yaml.Marshal(p)
	if err != nil {
		return false
	}
	if strings.Contains(string(configBytes), "beforeAction") {
		return true
	}
	return false
}
