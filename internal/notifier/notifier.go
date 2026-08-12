// internal/notifier/notifier.go
package notifier

import (
	"context"
)

// MessageType 通用消息类型 (基于 MIME 国际标准格式与分类)
type MessageType string

const (
	TypeText     MessageType = "text"     // 文本 / Markdown / HTML 富文本
	TypeImage    MessageType = "image"    // 所有图片与动图 (PNG / JPG / GIF / WebP)
	TypeVideo    MessageType = "video"    // 所有视频文件 (MP4 / AV1 / WebM)
	TypeDocument MessageType = "document" // 通用文档文件 (PDF / Excel / CSV / ZIP)
	TypeJSON     MessageType = "json"     // 结构化 JSON 数据 (对应 MIME application/json)
)

// Message 通用消息载体
type Message struct {
	Type     MessageType `json:"type"`                // 消息分类: text / image / video / document / json
	Content  string      `json:"content,omitempty"`   // 文本内容 (普通文本 / Markdown / HTML)
	FilePath string      `json:"file_path,omitempty"` // 本地文件路径 (图片 / 动图 / 视频 / PDF / Excel)
	Data     any         `json:"data,omitempty"`      // 结构化数据体 ( 交互卡片对象或 json.RawMessage)
	Target   string      `json:"target,omitempty"`    // 发送目标 (如特定 UserId / 群 ID，留空则发送给默认绑定目标)
}

// Notifier 接口定义
type Notifier interface {
	Push(ctx context.Context, msg Message) error
}
