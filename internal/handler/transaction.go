// internal/handler/transaction.go

package handler

import (
	"cmp"
	"log/slog"
	"net/http"
	"tallymind/internal/service"

	"github.com/gin-gonic/gin"
)

type TransactionHandler struct {
	accountService *service.AccountingService
}

func NewTransactionHandler(accountService *service.AccountingService) *TransactionHandler {
	return &TransactionHandler{accountService: accountService}
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
	userID := cmp.Or(c.GetHeader("X-User-ID"))
	sourceChannel := cmp.Or(c.GetHeader("X-Source-Channel"), "unknown_api_plugin")

	// Gin 的极简 JSON 绑定与自动校验
	var req service.BatchTransactions
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("API 请求 JSON 解析失败", "err", err)
		c.JSON(http.StatusBadRequest, APIResponse{
			Code:    400,
			Message: "请求数据格式错误，请检查 JSON 格式",
		})
		return
	}
	// 调用 ledger 处理函数
	replyData, err := h.accountService.RecordDirect(c.Request.Context(), userID, sourceChannel, &req)
	if err != nil {
		slog.Error("账单写入失败", "err", err)
		c.JSON(http.StatusBadRequest, APIResponse{
			Code:    400,
			Message: err.Error(),
		})
		return
	}

	finalUser := cmp.Or(userID, "unknown_user")
	finalChannel := cmp.Or(sourceChannel, "unknown_api_plugin")

	slog.Info("通用 API 成功记账", "user", finalUser, "channel", finalChannel, "headline", replyData.SummaryHeadline())
	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: replyData.SummaryHeadline(),
		Data: gin.H{
			"count":   replyData.Count,
			"total":   replyData.TotalAmount,
			"user":    finalUser,
			"channel": finalChannel,
			"details": req.Transactions,
		},
	})
}
