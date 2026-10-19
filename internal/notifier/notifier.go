// internal/notifier/notifier.go
package notifier

import (
	"context"
)

// MessageType 通用消息载体类型 (遵循 MIME / RFC 标准分类)
type MessageType string

const (
	TypeText     MessageType = "text"     // 纯文本 / Markdown
	TypeImage    MessageType = "image"    // 图片
	TypeVideo    MessageType = "video"    // 视频
	TypeDocument MessageType = "document" // 文档 / PDF / 报表文件
	TypeJSON     MessageType = "json"     // 原生结构化 JSON
)

// Message 通用跨平台消息实体
type Message struct {
	Type     MessageType       `json:"type"`                // 消息分类: text / image / video / document / json
	Title    string            `json:"title,omitempty"`     // 消息标题 (可选)
	Content  string            `json:"content,omitempty"`   // 正文 (普通文本 / Markdown)
	FilePath string            `json:"file_path,omitempty"` // 本地文件路径 (图片/视频/文档附件)
	Data     any               `json:"data,omitempty"`      // 结构化 Payload
	Target   string            `json:"target,omitempty"`    // 发送目标: 可以是特定人员 ID、群 ID，或覆盖的 Webhook URL
	Extra    map[string]string `json:"extra,omitempty"`     // 协议扩展元数据
}

// Notifier 统一通知驱动接口
type Notifier interface {
	Push(ctx context.Context, msg Message) error
}
