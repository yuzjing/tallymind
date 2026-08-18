// internal/ledger/beancount.go
package ledger

import (
	"cmp"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	zh_translations "github.com/go-playground/validator/v10/translations/zh"
)

var (
	validate = validator.New()
	trans    ut.Translator
)

func init() {
	// 初始化中文校验翻译器
	zhTrans := zh.New()
	uni := ut.New(zhTrans, zhTrans)
	trans, _ = uni.GetTranslator("zh")

	err := zh_translations.RegisterDefaultTranslations(validate, trans)
	if err != nil {
		slog.Error("RegisterDefaultTranslations 注册中文翻译失败", "err", err)
	}
}

// RequestContext 传输层与系统上下文
type RequestContext struct {
	UserID           string // 发消息人的UserId (如 "ZhangSan")
	SourceChannel    string // 消息渠道: text / voice / image
	Reporter         string // 映射的记账人别名 (如 "husband")
	DefaultAccount   string // 映射的默认微信卡 (如 "Assets:WeChat:Husband")
	DefaultCurrency  string // 默认币种 "CNY"
	FallbackCategory string // 默认兜底分类 "Expenses:Uncategorized"
	FallbackAccount  string // 默认兜底账户 "Assets:Pending:Unknown"
	FallbackPayee    string // 默认兜底商户名 (如 "日常消费")
}

// Metadata 扩展元数据 (包含固定字段 + Extra 无限动态扩展字典)
type Metadata struct {
	Reporter       string            `json:"reporter,omitempty"`        // 记账人/操作人 (如 husband)
	Link           string            `json:"link,omitempty"`            // 关联单号 (写为 ^order-12345)
	Time           string            `json:"time,omitempty"`            // 交易时间 (如 18:20:05)
	Location       string            `json:"location,omitempty"`        // 地点
	Beneficiary    string            `json:"beneficiary,omitempty"`     // 受益人 (如 family/baby)
	InvoiceStatus  string            `json:"invoice_status,omitempty"`  // 发票状态 (pending/done)
	SourceChannel  string            `json:"source_channel,omitempty"`  // 渠道 (text/voice/image)
	OriginalAmount string            `json:"original_amount,omitempty"` // 优惠前原价
	DiscountAmount string            `json:"discount_amount,omitempty"` // 优惠金额
	Extra          map[string]string `json:"extra,omitempty"`           // 动态扩展字典，支持任意自定义元数据
}

// Transaction 核心交易结构体
type Transaction struct {
	Amount    float64   `json:"amount" validate:"required,gt=0"`               // 金额必填且必须 > 0
	Date      string    `json:"date" validate:"omitempty,datetime=2006-01-02"` // 交易日期 YYYY-MM-DD
	Payee     string    `json:"payee" validate:"omitempty,max=500"`            // 商户名 (上限 500 字符)
	Narration string    `json:"narration" validate:"omitempty,max=1000"`       // 详细备注 (上限 1000 字符，可为空)
	Category  string    `json:"category"`                                      // 支出/收入科目
	Account   string    `json:"account"`                                       // 支付/资产账户
	Currency  string    `json:"currency,omitempty"`                            // 货币 (默认 CNY)
	Type      string    `json:"type,omitempty"`                                // 交易类型: expense / income / refund
	Tags      []string  `json:"tags,omitempty"`                                // 标签 (如 ["#公司报销"])
	Meta      *Metadata `json:"metadata,omitempty"`                            // 扩展元数据
}

// BatchTransactions 批量交易请求体
type BatchTransactions struct {
	Transactions []Transaction `json:"transactions" validate:"required,divby=1"`
}

// Validate 全字段硬性保底校验
func (t *Transaction) Validate() error {
	err := validate.Struct(t)
	if err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			for _, fieldErr := range validationErrs {
				slog.Warn("账单数据校验失败", "err", err)
				return fmt.Errorf("记账失败: %s", fieldErr.Translate(trans)) // 返回中文提示
			}
		}
		return fmt.Errorf("记账失败: %w", err)
	}
	return nil
}

// EnsureDefaults 极简降级兜底 (采用 Go 1.21+ cmp.Or 与 slices.Contains)
func (t *Transaction) EnsureDefaults(ctx RequestContext) {
	// ⭐️ 第一行强效防空指针：如果 Meta 是 nil，立刻给它初始化一个空的 &Metadata{} 结构体！
	if t.Meta == nil {
		t.Meta = &Metadata{}
	}

	// 1. 基础字段降级
	t.Date = cmp.Or(t.Date, time.Now().Format("2006-01-02"))
	t.Currency = cmp.Or(t.Currency, ctx.DefaultCurrency)
	t.Payee = cmp.Or(t.Payee, ctx.FallbackPayee)

	needsReview := false

	// 2. 科目检查与降级
	if !hasStandardPrefix(t.Category) {
		slog.Debug("科目格式不合规，自动降级", "raw_category", t.Category, "fallback", ctx.FallbackCategory)
		t.Category = ctx.FallbackCategory
		needsReview = true
	}

	// 3. 账户检查与降级
	if !hasStandardPrefix(t.Account) {
		if ctx.DefaultAccount != "" {
			t.Account = ctx.DefaultAccount
		} else {
			t.Account = ctx.FallbackAccount
			needsReview = true
		}
	}

	// 4. 微信上下文元数据填充
	t.Meta.SourceChannel = cmp.Or(t.Meta.SourceChannel, ctx.SourceChannel)
	t.Meta.Reporter = cmp.Or(t.Meta.Reporter, ctx.Reporter)

	// 5. 追加复核标签 (用 slices.Contains 简化，消除多行循环)
	if needsReview && !slices.Contains(t.Tags, "#needs-review") {
		t.Tags = append(t.Tags, "#needs-review")
	}
}

// 负责把盒子里的每笔交易拿出来调上面的方法
// =========================================================================
func (b *BatchTransactions) EnsureDefaults(ctx RequestContext) {
	for i := range b.Transactions {
		// 逐笔调用
		b.Transactions[i].EnsureDefaults(ctx)
	}
}

// ToBeancountFormat 转换为标准 Beancount 纯文本字符串 (使用 fmt.Fprintf 直写内存缓冲区，性能最高)
func (t *Transaction) ToBeancountFormat() string {
	var builder strings.Builder

	// 1. 处理 Tags 与 Link 拼接
	tagString := ""
	if len(t.Tags) > 0 {
		tagString = " " + strings.Join(t.Tags, " ")
	}
	linkString := ""
	if t.Meta.Link != "" {
		linkString = " ^" + t.Meta.Link
	}

	// 2. 备注处理：有备注才拼双引号，无备注直接留空
	narrationPart := ""
	if t.Narration != "" {
		narrationPart = fmt.Sprintf(" \"%s\"", t.Narration)
	}

	// 首行：2026-08-06 * "商户" "备注" #标签 ^链接
	fmt.Fprintf(&builder, "%s * \"%s\"%s%s%s\n", t.Date, t.Payee, narrationPart, tagString, linkString)

	// 3. 反射：自动遍历 Metadata 中的所有预设结构体字段
	metaVal := reflect.ValueOf(*t.Meta)
	metaType := reflect.TypeFor[Metadata]()

	for i := 0; i < metaVal.NumField(); i++ {
		structField := metaType.Field(i)
		fieldVal := metaVal.Field(i)

		// 跳过 Extra 字典和 Link 字段（Link 已在首行 ^link 处理）
		if structField.Name == "Extra" || structField.Name == "Link" {
			continue
		}

		// 从 json tag 提取键名
		jsonTag := structField.Tag.Get("json")
		keyName := strings.Split(jsonTag, ",")[0]
		if keyName == "" || keyName == "-" {
			keyName = strings.ToLower(structField.Name)
		}

		// 直接用 Fprintf 写入 builder，避免生成临时字符串
		if fieldVal.Kind() == reflect.String && fieldVal.String() != "" {
			fmt.Fprintf(&builder, "  %s: \"%s\"\n", keyName, fieldVal.String())
		}
	}

	// 4. 遍历 Extra 动态扩展字典 (直接用 Fprintf 写入 builder)
	for k, v := range t.Meta.Extra {
		if k != "" && v != "" {
			fmt.Fprintf(&builder, "  %s: \"%s\"\n", k, v)
		}
	}

	// 5. 正负号处理
	absAmount := math.Abs(t.Amount)
	categoryAmount := absAmount
	accountAmount := -absAmount
	if t.Type == "refund" || t.Type == "income" {
		categoryAmount = -absAmount
		accountAmount = absAmount
	}

	// 6. 科目借贷行
	fmt.Fprintf(&builder, "  %s  %.2f %s\n  %s  %.2f %s\n\n",
		t.Category, categoryAmount, t.Currency,
		t.Account, accountAmount, t.Currency,
	)

	return builder.String()
}

// AppendBatchTransactions 自动按年份拆分文件并追加写入
func AppendBatchTransactions(basePath string, req BatchTransactions, ctx RequestContext) error {
	// 按年份分组存储 Beancount 文本: map[年份文件路径]文本内容
	yearlyTextMap := make(map[string]*strings.Builder)

	for _, tx := range req.Transactions {
		// 1. 校验与补全默认值
		if err := tx.Validate(); err != nil {
			return err
		}
		tx.EnsureDefaults(ctx)

		// 2. 根据交易日期推导目标年份文件路径 (如: transactions/2026.bean)
		targetPath := GetYearlyFilePath(basePath, tx.Date)

		// 3. 按年份文件归类文本
		if _, exists := yearlyTextMap[targetPath]; !exists {
			yearlyTextMap[targetPath] = &strings.Builder{}
		}
		yearlyTextMap[targetPath].WriteString(tx.ToBeancountFormat())
	}

	// 4. 遍历年份 map，分别追加写入对应的 .bean 文件
	for targetPath, builder := range yearlyTextMap {

		// ⭐️ 自动创建父级文件夹 (例如 transactions/ 目录不存在时自动建好)
		dir := filepath.Dir(targetPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建账本目录失败 [%s]: %w", dir, err)
		}

		file, err := os.OpenFile(targetPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("打开/创建账单文件失败 [%s]: %w", targetPath, err)
		}

		_, err = file.WriteString(builder.String())
		file.Close() // 及时关闭文件
		if err != nil {
			return fmt.Errorf("写入账单文件失败 [%s]: %w", targetPath, err)
		}
		slog.Info("成功追加账单到文件", "path", targetPath)
	}

	return nil
}

// GetYearlyFilePath 辅助函数：根据交易日期推导年份文件路径
func GetYearlyFilePath(basePath string, dateStr string) string {
	dir := filepath.Dir(basePath)
	year := time.Now().Format("2006")

	parts := strings.Split(dateStr, "-")
	if len(parts) > 0 && len(parts[0]) == 4 {
		year = parts[0]
	}

	return filepath.Join(dir, fmt.Sprintf("%s.bean", year))
}

// 辅助函数：判断是不是以 Beancount 五大标准前缀开头
func hasStandardPrefix(account string) bool {
	return strings.HasPrefix(account, "Expenses:") ||
		strings.HasPrefix(account, "Assets:") ||
		strings.HasPrefix(account, "Income:") ||
		strings.HasPrefix(account, "Liabilities:") ||
		strings.HasPrefix(account, "Equity:")
}

// ToSummaryString 生成给家人回复的友情账单摘要文本
func (b *BatchTransactions) ToSummaryString() string {
	if len(b.Transactions) == 0 {
		return "⚠️ 未能识别到有效的记账信息"
	}

	var builder strings.Builder
	builder.WriteString("✅ 成功为你记录账单：\n")

	for i, tx := range b.Transactions {
		if i > 0 {
			builder.WriteString("\n┈┈┈┈┈┈┈┈┈┈┈┈┈┈\n")
		}
		// 拼接精美的文本卡片
		fmt.Fprintf(&builder, "💰 金额：%.2f %s\n", tx.Amount, tx.Currency)
		fmt.Fprintf(&builder, "📂 分类：%s\n", tx.Category)
		fmt.Fprintf(&builder, "📅 日期：%s\n", tx.Date)
		if tx.Payee != "" {
			fmt.Fprintf(&builder, "🏪 商户：%s\n", tx.Payee)
		}
		if tx.Narration != "" {
			fmt.Fprintf(&builder, "📝 备注：%s\n", tx.Narration)
		}
		if tx.Account != "" {
			fmt.Fprintf(&builder, "💳 账户：%s\n", tx.Account)
		}
	}

	return builder.String()
}
