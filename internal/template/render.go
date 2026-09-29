package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/pelletier/go-toml/v2"
)

// Render 读取并渲染 TOML 声明式模板，通过 JSON 桥接泛型反序列化为目标结构体 T
func Render[T any](templateDir, templateRelativePath string, data any) (*T, error) {
	fullPath := filepath.Join(templateDir, templateRelativePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read template file [%s] failed: %w", fullPath, err)
	}

	// 1. Go template 动态变量渲染
	tmpl, err := template.New(filepath.Base(fullPath)).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template file [%s] failed: %w", fullPath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template file [%s] failed: %w", fullPath, err)
	}

	// 2. 反序列化 TOML 为通用 Map 结构
	var rawObj any
	if err := toml.Unmarshal(buf.Bytes(), &rawObj); err != nil {
		return nil, fmt.Errorf("unmarshal toml failed: %w\n--- RAW OUTPUT ---\n%s\n------------------", err, buf.String())
	}

	// 3. ⭐️ 核心桥接：转为 JSON 再落到目标结构体
	// 优势：所有上游消息结构体只需声明原生的 `json:"..."` 标签，天然支持微信等平台的下发契约
	jsonBytes, err := json.Marshal(rawObj)
	if err != nil {
		return nil, fmt.Errorf("marshal intermediate json [%s] failed: %w", fullPath, err)
	}

	var result T
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("unmarshal target struct [%s] failed: %w", fullPath, err)
	}

	return &result, nil
}
