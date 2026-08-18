// internal/template/render.go
package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Render 读取并渲染 YAML 模板，反序列化为目标结构体 T
func Render[T any](templateDir, templateRelativePath string, data any) (*T, error) {
	fullPath := filepath.Join(templateDir, templateRelativePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read template file [%s] failed: %w", fullPath, err)
	}

	// 注册 Helm 同款通用辅助函数库
	funcMap := template.FuncMap{
		"indent":  indent,
		"nindent": nindent,
		"quote":   quote,
	}

	// 1. Go template 动态变量替换

	tmpl, err := template.New(filepath.Base(fullPath)).Funcs(funcMap).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template file [%s] failed: %w", fullPath, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template file [%s] failed: %w", fullPath, err)
	}

	// 2. 可跳过技巧：K8S等使用的yaml文件先解为通用 Map，再转为 JSON
	// 这样具体插件的handler中可以不用写成`json:"duplication_check_interval,omitempty" yaml:"duplication_check_interval,omitempty"`

	var rawObj any
	if err := yaml.Unmarshal(buf.Bytes(), &rawObj); err != nil {
		return nil, fmt.Errorf("unmarshal yaml failed: %w\n--- RAW OUTPUT ---\n%s\n------------------", err, buf.String())
	}
	jsonBytes, err := json.Marshal(rawObj)
	if err != nil {
		return nil, fmt.Errorf("marshal yaml [%s] failed: %w", fullPath, err)
	}

	// 3. 最终通过结构体原有的 json 标签反序列化

	var result T
	if err := yaml.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("unmarshal json [%s] failed: %w", fullPath, err)
	}

	return &result, nil
}

// indent 将多行文本的每一行前面补齐指定数量的空格
func indent(spaces int, v any) string {
	pad := strings.Repeat(" ", spaces)
	s := fmt.Sprintf("%v", v)
	return strings.ReplaceAll(s, "\n", "\n"+pad)

}

// nindent 先另起一行，再对每一行补齐指定数量的空格
func nindent(spaces int, v any) string {
	pad := strings.Repeat(" ", spaces)
	s := fmt.Sprintf("%v", v)
	return "\n" + strings.ReplaceAll(s, "\n", "\n"+pad)
}

// quote 给单行字符串自动加上双引号并做转义防爆
func quote(v string) string {
	return fmt.Sprintf("\"%s\"", strings.ReplaceAll(v, `"`, `\"`))
}
