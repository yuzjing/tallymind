// internal/ledger/format.go
package ledger

import (
	"fmt"
	"math"
	"reflect"
	"strings"
)

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
