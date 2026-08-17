// internal/template/render.go
package template

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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

	tmpl, err := template.New(filepath.Base(fullPath)).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template file [%s] failed: %w", fullPath, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template file [%s] failed: %w", fullPath, err)
	}

	var result T
	if err := yaml.Unmarshal(buf.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("unmarshal template file [%s] failed: %w", fullPath, err)
	}

	return &result, nil
}
