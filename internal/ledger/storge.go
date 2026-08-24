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
	// 按年份分组存储 Beancount 文本: map[年份文件路径]文本内容
	yearlyTextMap := make(map[string]*strings.Builder)

	req.EnsureDefaults(cfg, ctx)

	for _, tx := range req.Transactions {
		// 1. 校验与补全默认值
		if err := tx.Validate(); err != nil {
			return err
		}

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
