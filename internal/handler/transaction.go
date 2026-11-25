// internal/handler/transaction.go

package handler

import (
	"cmp"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"tallymind/internal/service"

	"github.com/gin-gonic/gin"
)

type TransactionHandler struct {
	accountService *service.AccountingService
	apiToken       string
}

func NewTransactionHandler(accountService *service.AccountingService, apiToken string) *TransactionHandler {
	return &TransactionHandler{
		accountService: accountService,
		apiToken:       strings.TrimSpace(apiToken),
	}
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
	// 若配置了 app.api_token，则强制执行常量时间比对校验 (兼容 Authorization: Bearer 或 X-API-Key)
	if h.apiToken != "" {
		provided := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if provided == "" {
			if auth := c.GetHeader("Authorization"); len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
				provided = strings.TrimSpace(auth[7:])
			}
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(h.apiToken)) != 1 {
			c.JSON(http.StatusUnauthorized, APIResponse{
				Code:    401,
				Message: "未授权的 API 请求",
			})
			return
		}
	}

	// 获取请求header
	userID := strings.TrimSpace(c.GetHeader("X-User-ID"))
	sourceChannel := cmp.Or(strings.TrimSpace(c.GetHeader("X-Source-Channel")), "unknown_api_plugin")

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
