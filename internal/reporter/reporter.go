// internal/reporter/reporter.go
package reporter

import (
	"fmt"
	"time"

	"tallymind/internal/notifier"
)

// GenerateMockReport 纯粹产出 Mock 测试简报 (只管生成数据，不负责发送！)
func GenerateMockReport() notifier.Message {
	return notifier.Message{
		Type: notifier.TypeText,
		Content: fmt.Sprintf("### 📊 tallymind 管道连通性测试 (MVP)\n"+
			"- **测试状态**：`成功连通`\n"+
			"- **发送时间**：`%s`", time.Now().Format("2006-01-02 15:04:05")),
	}
}
