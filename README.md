# tallymind

基于 Go + Beancount + 大模型的通用自动化复式记账微服务。

可通过聊天机器人（长连接）、工作流 API 插件或 Webhook 接收消费描述（文本/语音/图片），利用大模型（DeepSeek / Qwen / OpenAI）提炼交易要素，自动追加写入本地 Beancount 纯文本账本（`.bean`）。

## 核心特性

- **多通道灵活接入**：支持 WebSocket 长连接模式（内网/NAS 零公网部署、免域名备案）与 HTTP REST API 模式（自带 Swagger UI，可直接作为 Dify、Coze、企微/飞书工作流插件）。
- **大模型精准提取**：兼容 OpenAI 标准 API 格式，支持在 `.env` 中自由切换 DeepSeek、通义千问或本地 Ollama 私有模型。
- **数据主权完全自主**：数据直接存储于本地 `.bean` 纯文本文件，支持按年份（如 `transactions/2026.bean`）自动拆分归档。
- **高可用与防御性设计**：采用声明式校验（`validator/v10`）与 `cmp.Or` 降级兜底，异常格式自动打上 `#needs-review` 标签，确保服务 24/7 稳定运行。
- **解耦插件化架构**：账本核心引擎、LLM 解析器与消息网关彻底解耦，支持轻松扩展新的消息通道（企微、飞书、Telegram 等）。

## 系统架构

```text
[ 聊天机器人 / 外部 API / 工作流 / AI Agent ]
                      │
               (WSS / HTTP POST)
                      │
                      ▼
             [ tallymind (Go) ]
                      │
 ┌────────────────────┼────────────────────┐
 ▼                    ▼                    ▼
[ internal/llm ]   [ internal/ledger ]  [ internal/handler ]
(大模型提取 JSON)  (Beancount 校验与落盘) (Swagger / REST API)
 │                    │
 ▼                    ▼
[ 结构化 JSON ]   [ transactions/2026.bean ]
```

## 目录结构

```text
.
├── cmd/tallymind/         # 主服务启动入口
├── internal/
│   ├── config/            # 12-Factor 环境变量加载与子配置隔离
│   ├── ledger/            # Beancount 账本引擎 (校验、降级、反射格式化)
│   ├── llm/               # 兼容 OpenAI 标准的 LLM 客户端
│   ├── handler/           # 通用 HTTP REST API 处理器
│   ├── reporter/          # 报表调度器 (可集成 Python 数据分析/画图)
│   ├── notifier/          # 跨渠道消息推送接口规范
│   └── plugin/            # 消息渠道插件目录 (包含企微/预留 Telegram/飞书等)
├── scripts/               # 外部分析与画图脚本 (Python)
├── docs/                  # swag 自动生成的 Swagger 文档
└── .env.example           # 环境变量配置模板
```

## 快速开始

### 1. 克隆与配置

```bash
cp .env.example .env
# 编辑 .env 配置相关环境变量
```

### 2. 本地运行

```bash
go run ./cmd/tallymind
```

### 3. 生成 Swagger 文档

```bash
go run github.com/swaggo/swag/cmd/swag@latest init -g cmd/tallymind/main.go --parseInternal
```
访问地址：`http://localhost:8080/swagger/index.html`

## 环境变量说明

| 变量名 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `APP_ENV` | `development` | 运行环境 (`development` / `production`) |
| `ENABLE_WECOM_WSS` | `true` | 是否开启企微 WSS 长连接服务 |
| `ENABLE_HTTP_API` | `true` | 是否开启通用 HTTP API 服务 |
| `ENABLE_LLM` | `true` | 是否启用大模型解析 |
| `BEANCOUNT_FILE_PATH` | `main.bean` | Beancount 账本主文件路径 |
| `DEFAULT_CURRENCY` | `CNY` | 默认货币 |
| `DEFAULT_REPORTER` | `husband` | 默认记账人 |
| `LLM_BASE_URL` | `https://api.deepseek.com/v1` | LLM API 地址 |
| `LLM_MODEL` | `deepseek-chat` | LLM 模型名称 |

