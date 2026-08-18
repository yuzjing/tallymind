// plugin/wecom/message.go (纯净版，零 yaml 标签污染！)
package wecom

// MessageRequest 企微全能统一消息模型 (HTTP 自建应用 与 WSS 长连接机器人共用)
type MessageRequest struct {
	// ==================== 1. HTTP 专用路由与控制字段 (WSS 模式留空自动 omitempty 忽略) ====================
	ToUser                 string `json:"touser,omitempty"`                   // 成员ID列表 (多个用|分隔，@all表示全员)
	ToParty                string `json:"toparty,omitempty"`                  // 部门ID列表 (多个用|分隔)
	ToTag                  string `json:"totag,omitempty"`                    // 标签ID列表 (多个用|分隔)
	AgentID                int64  `json:"agentid,omitempty"`                  // 企业应用的 AgentID
	EnableDuplicateCheck   int    `json:"enable_duplicate_check,omitempty"`   // 是否开启重复消息检查: 0=否, 1=是
	DuplicateCheckInterval int    `json:"duplicate_check_interval,omitempty"` // 去重时间间隔 (默认 1800 秒，最大 14400 秒)

	// ==================== 2. 通用消息类型定义 (必填) ====================
	MsgType string `json:"msgtype"` // text / markdown / image / voice / video / file / news / template_card

	// ==================== 3. 各种媒体类型的内容载体 (指针 + omitempty 保证零冗余) ====================
	Text         *TextContent         `json:"text,omitempty"`
	Markdown     *MarkdownContent     `json:"markdown,omitempty"`
	Image        *MediaContent        `json:"image,omitempty"`
	Voice        *MediaContent        `json:"voice,omitempty"`
	Video        *VideoContent        `json:"video,omitempty"`
	File         *MediaContent        `json:"file,omitempty"`
	News         *NewsContent         `json:"news,omitempty"`
	TemplateCard *TemplateCardContent `json:"template_card,omitempty"`
}

type TextContent struct {
	Content string `json:"content"`
}

type MarkdownContent struct {
	Content string `json:"content"`
}

type MediaContent struct {
	MediaID string `json:"media_id"`
}

type VideoContent struct {
	MediaID     string `json:"media_id"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type NewsContent struct {
	Articles []NewsArticle `json:"articles"`
}

type NewsArticle struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
	PicURL      string `json:"picurl,omitempty"`
}

type TemplateCardContent struct {
	// ==================== 1. 公用基础字段 ====================
	CardType string `json:"card_type"` // "text_notice" 或 "news_notice"

	// 主标题信息 (两种卡片均支持 title 和 desc)
	MainTitle struct {
		Title string `json:"title"`          // 一级标题
		Desc  string `json:"desc,omitempty"` // 标题辅助信息/描述
	} `json:"main_title"`

	// 卡片底部跳转行为 (两种卡片均必填)
	CardAction struct {
		Type string `json:"type"` // 1: 跳转 URL, 2: 打开小程序
		URL  string `json:"url,omitempty"`
	} `json:"card_action"`

	// ==================== 2. 文本通知型 (text_notice) 专属字段 ====================

	// 仅 text_notice 支持：二级副标题
	SubTitleText string `json:"sub_title_text,omitempty"`

	// 仅 text_notice 支持：关键数据突显 (如大号字体显示 "￥500.00")
	EmphasisContent *struct {
		Title string `json:"title"`          // 突显数值
		Desc  string `json:"desc,omitempty"` // 数值描述
	} `json:"emphasis_content,omitempty"`

	// 仅 text_notice 支持：左右键值对列表 (如 "分类: 餐饮")
	HorizontalContentList []struct {
		KeyName string `json:"keyname"`
		Value   string `json:"value"`
	} `json:"horizontal_content_list,omitempty"`

	// ==================== 3. 图文展示型 (news_notice) 专属字段 ====================

	// 仅 news_notice 支持：顶部大图 (CardImage 和 ImageTextArea 必须二选一填一个)
	CardImage *struct {
		URL         string  `json:"url"`                    // 图片 URL
		AspectRatio float64 `json:"aspect_ratio,omitempty"` // 宽高比，默认 1.3
	} `json:"card_image,omitempty"`

	// 仅 news_notice 支持：左图右文摘要区
	ImageTextArea *struct {
		Type  int    `json:"type,omitempty"`  // 1: 跳转 URL
		URL   string `json:"url,omitempty"`   // 跳转地址
		Title string `json:"title,omitempty"` // 标题
		Desc  string `json:"desc,omitempty"`  // 描述
	} `json:"image_text_area,omitempty"`

	// 仅 news_notice 支持：卡片下方垂直列表项 (如文章/明细推荐)
	VerticalContentList []struct {
		Title string `json:"title"`          // 列表标题
		Desc  string `json:"desc,omitempty"` // 列表描述
	} `json:"vertical_content_list,omitempty"`

	// 仅 news_notice 支持：底部多链接跳转列表 (最多 3 个)
	JumpList []struct {
		Type  int    `json:"type"`  // 1: 跳转 URL
		Title string `json:"title"` // 链接文案
		URL   string `json:"url"`   // 链接地址
	} `json:"jump_list,omitempty"`
}

// 工厂函数：
func NewTextMessage(toUser string, content string) *MessageRequest {
	return &MessageRequest{
		ToUser:  toUser,
		MsgType: "text",
		Text:    &TextContent{Content: content},
	}
}

func NewMarkdownMessage(toUser string, content string) *MessageRequest {
	return &MessageRequest{
		ToUser:   toUser,
		MsgType:  "markdown",
		Markdown: &MarkdownContent{Content: content},
	}
}

func NewTemplateCardMessage(toUser string, card *TemplateCardContent) *MessageRequest {
	return &MessageRequest{
		ToUser:       toUser,
		MsgType:      "template_card",
		TemplateCard: card,
	}
}
