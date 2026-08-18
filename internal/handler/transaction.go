// internal/handler/transaction.go

package handler

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"tallymind/internal/config"
	"tallymind/internal/ledger"

	"github.com/gin-gonic/gin"
)

// API通用结构体
type APIResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type TransactionHandler struct {
	cfg config.LedgerConfig
}

func NewTransactionHandler(ledgerCfg config.LedgerConfig) *TransactionHandler {
	return &TransactionHandler{cfg: ledgerCfg}
}

// HandleTransaction 通用 HTTP 记账 API 接口
// @Summary      通用记账 API 接口
// @Description  接收标准 JSON 账单数据，校验后追加写入本地 Beancount 纯文本账本
// @Tags         账单管理
// @Accept       json
// @Produce      json
// @Param        X-User-ID         header    string                    false  "记账人 ID (如 husband)"
// @Param        X-Source-Channel  header    string                    false  "渠道来源 (如 wecom_api_plugin)"
// @Param        request           body      ledger.BatchTransactions  true   "账单请求体"
// @Success      200               {object}  APIResponse               "成功返回"
// @Failure      400               {object}  APIResponse               "参数校验或保存失败"
// @Router       /api/v1/transaction [post]
func (h *TransactionHandler) HandleTransaction(c *gin.Context) {
	// 获取请求header
	userID := cmp.Or(c.GetHeader("X-User-ID"), h.cfg.DefaultReporter)
	sourceChannel := cmp.Or(c.GetHeader("X-Source-Channel"), "unknown_api_plugin")

	// 组装请求体传输层上下文
	reqCtx := ledger.RequestContext{
		UserID:           userID,
		Reporter:         userID,
		SourceChannel:    sourceChannel,
		DefaultCurrency:  h.cfg.DefaultCurrency,
		FallbackCategory: h.cfg.FallbackCategory,
		FallbackAccount:  h.cfg.FallbackAccount,
	}

	// Gin 的极简 JSON 绑定与自动校验
	var req ledger.BatchTransactions
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("API 请求 JSON 解析失败", "err", err)
		c.JSON(http.StatusBadRequest, APIResponse{
			Code:    400,
			Message: "请求数据格式错误，请检查 JSON 格式",
		})
		return
	}
	// 调用 ledger 处理函数
	if err := ledger.AppendBatchTransactions(h.cfg.FilePath, req, reqCtx); err != nil {
		slog.Error("账单写入失败", "err", err)
		c.JSON(http.StatusBadRequest, APIResponse{
			Code:    400,
			Message: err.Error(),
		})
		return
	}

	//生成详细的友情摘要卡片 (包含日期、商户、备注、金额、分类、账户)
	summaryText := req.ToSummaryString()

	// 返回成功响应
	slog.Info("通用 API 成功记账", "user", userID, "channel", sourceChannel, "summary", summaryText)
	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: summaryText,
		Data: gin.H{
			"count":   len(req.Transactions),
			"user":    userID,
			"channel": sourceChannel,
			"details": req.Transactions, // 同时也把原生数组塞进 Data 里
		},
	})
}

func (h *TransactionHandler) SaveBatch(ctx context.Context, userID string, sourceChannel string, batch *ledger.BatchTransactions) (string, error) {
	if batch == nil || len(batch.Transactions) == 0 {
		return "", fmt.Errorf("交易批次为空，无需存盘")
	}

	// 存盘上下文在 transaction.go 内部组装
	reqCtx := ledger.RequestContext{
		UserID:           userID,
		Reporter:         userID,
		SourceChannel:    sourceChannel,
		DefaultCurrency:  h.cfg.DefaultCurrency,
		FallbackCategory: h.cfg.FallbackCategory,
		FallbackAccount:  h.cfg.FallbackAccount,
	}
	batch.EnsureDefaults(reqCtx)

	if err := ledger.AppendBatchTransactions(h.cfg.FilePath, *batch, reqCtx); err != nil {
		return "", fmt.Errorf("保存交易批次失败: %w", err)
	}

	return batch.ToSummaryString(), nil
}
