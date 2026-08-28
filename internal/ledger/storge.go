// internal/ledger/storge.go
package ledger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AppendBatchTransactions 自动按年份拆分文件并追加写入
func AppendBatchTransactions(basePath string, req *BatchTransactions, cfg Config, ctx RequestContext) error {
	yearlyTextMap := make(map[string]*strings.Builder)

	req.EnsureDefaults(cfg, ctx)

	// 1. 遍历写入常规交易行
	for _, tx := range req.Transactions {
		if err := tx.Validate(); err != nil {
			return err
		}

		targetPath := GetYearlyFilePath(basePath, tx.Date)
		if _, exists := yearlyTextMap[targetPath]; !exists {
			yearlyTextMap[targetPath] = &strings.Builder{}
		}
		yearlyTextMap[targetPath].WriteString(tx.ToBeancountFormat())
	}

	// 2. 遍历写入资产断言与自动找平指令 (balance / pad)
	for _, b := range req.BalanceAssertions {
		if b.Account == "" {
			continue
		}
		if b.Date == "" {
			b.Date = ctx.MessageTime.Format("2006-01-02")
		}

		targetPath := GetYearlyFilePath(basePath, b.Date)
		if _, exists := yearlyTextMap[targetPath]; !exists {
			yearlyTextMap[targetPath] = &strings.Builder{}
		}
		yearlyTextMap[targetPath].WriteString(b.ToBeancountFormat(cfg))
	}

	// 3. 遍历年份 map，分别追加写入对应的 .bean 文件
	for targetPath, builder := range yearlyTextMap {
		dir := filepath.Dir(targetPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建账本目录失败 [%s]: %w", dir, err)
		}

		file, err := os.OpenFile(targetPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("打开/创建账单文件失败 [%s]: %w", targetPath, err)
		}

		_, err = file.WriteString(builder.String())
		_ = file.Close()
		if err != nil {
			return fmt.Errorf("写入账单文件失败 [%s]: %w", targetPath, err)
		}
		slog.Info("成功追加账单/断言到文件", "path", targetPath)
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