// internal/ledger/validate.go
package ledger

import (
	"fmt"
	"log/slog"
	"strings"

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

// Validate 全字段硬性保底校验 (Transaction 实体校验)
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

// Validate 资产断言合法性校验 (⭐️ 新增 BalanceAssertion 校验)
func (b *BalanceAssertion) Validate() error {
	if strings.TrimSpace(b.Account) == "" {
		return fmt.Errorf("资产对账账户不能为空")
	}
	if b.Amount < 0 {
		return fmt.Errorf("资产断言金额不能为负数")
	}
	return nil
}