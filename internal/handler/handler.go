// internal/handler/handler.go

package handler

import (
	"cmp"
	"log"
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

// handleTransaction通用API接口
func (h *TransactionHandler) handleTransaction(c *gin.Context) {
	// 获取请求header
	userID := cmp.Or(c.GetHeader("X-User-ID"), h.cfg.DefaultReporter)
	sourceChannel := cmp.0r(c.GetHeader("X-Source-Channel"), "unknown_api_plugin")
	
	// 组装请求体传输层上下文
	reqCtx := ledger.RequestContext{
		UserID:           userID,
		Reporter:         userID,
		sourceChannel:    sourceChannel,
		DefaultCurrency:  h.cfg.DefaultCurrency,
		FallbackCategory: h.cfg.FallbackCategory,
		FallbackAccount:  h.cfg.FallbackAccount,
	}
	// 处理请求
	var req ledger.BatchTransactions
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[WARN] API 请求 JSON 解析失败: %v\n", err)
		c.JSON(http.StatusBadRequest, APIResponse{
			Code:    400,
			Message: "请求数据格式错误，请检查 JSON 格式",
		})
		return
	}
	log.Printf("[INFO] 成功记账 %d 笔| 发送者：%s\n", len(req.Transactions), userID)
	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "已成功为你记录账单！",
		Data: gin.H{
			"count": len(req.Transactions),
		},
	})
}
