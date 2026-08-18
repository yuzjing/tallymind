# 🌿 TallyMind

[English](#english) | [中文](#chinese)

---

<a name="english"></a>
## English

**TallyMind** is an automated, multimodal bookkeeping agent written in Go. It transforms natural language text and receipt screenshots into strict, plain-text [Beancount](https://beancount.github.io/) double-entry accounting records, interfacing seamlessly with WeChat and Enterprise WeChat (WeCom).

[![Go Report Card](https://goreportcard.com/badge/github.com/yuzjing/tallymind)](https://goreportcard.com/report/github.com/yuzjing/tallymind)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Container Image](https://img.shields.io/badge/GHCR-tallymind-blue?logo=docker)](https://github.com/yuzjing/tallymind/pkgs/container/tallymind)

### 🚀 Key Features

- **Multimodal AI Pipeline**: Parses complex multi-item text and receipt images into structured JSON via an OpenAI-compatible Vision API (e.g., Gemini, DeepSeek, Qwen-VL).
- **Plain-Text Accounting (PTA)**: Appends standardized transactions directly to local `.bean` files, fully compatible with `Fava` and `lazybeancount`.
- **Dynamic YAML/Go Template Engine**: Message formats are decoupled from source code; templates support runtime hot-reloading without container restarts.
- **Decoupled Architecture**: Follows Hexagonal / Ports & Adapters principles to isolate the core ledger domain from inbound webhooks and outbound notification channels.
- **Dual-Client Compatibility**: Automatically formats receipts for both Enterprise WeChat and Personal WeChat (via WeChat Work Plugin).

### 🏗️ Architecture

```text
[WeChat / WeCom Webhook]
           │
           ▼
[Inbound Callback Handler] (Decryption & 200 OK Fast-Ack)
           │
           ├─► (Async Worker Goroutine)
           ▼
[Core Transaction Pipeline]
   ├──► 1. Multimodal LLM (Text / Image OCR -> Structured JSON)
   ├──► 2. Ledger Engine (Validation & Beancount Append)
   └──► 3. Template Engine (YAML / Text Rendering -> Outbound Dispatch)
```

### ⚡ Quick Start

#### 1. Configuration (`.env`)

```env
# Server & Logging
APP_ENV=production
LOG_LEVEL=info
SERVER_PORT=8080
PUBLIC_URL=yourdomain
TEMPLATE_DIR=templates

# Feature Flags
ENABLE_HTTP_API=true
ENABLE_LLM=true
ENABLE_WECOM_HTTP=true
ENABLE_REPORTER=false

# LLM (OpenAI-Compatible / Gemini API)
LLM_API_KEY=your_api_key
LLM_BASE_URL=https://api.openai.com/v1
LLM_MODEL=gpt-4

# WeCom Credentials
WECOM_CORP_ID=ww_your_corpid
WECOM_AGENT_ID=1000002
WECOM_SECRET=your_app_secret
WECOM_TOKEN=your_callback_token
WECOM_ENCODING_AES_KEY=your_43_chars_encoding_aes_key
WECOM_EXPENSE_TEMPLATE=wecom/expense_success.yaml
WECOM_EXPENSE_FAILURE_TEMPLATE=wecom/expense_failure.yaml

# Ledger
BEANCOUNT_FILE_PATH=data/2026.bean
DEFAULT_CURRENCY=CNY
DEFAULT_REPORTER=User
FALLBACK_CATEGORY=Expenses:Uncategorized
FALLBACK_ACCOUNT=Assets:Pending:Unknown
```

#### 2. Run with Docker / Podman

```bash
docker run -d \
  --name tallymind \
  --restart always \
  -p 8080:8080 \
  -v ./data:/app/data \
  -v ./templates:/app/templates:ro \
  --env-file .env \
  ghcr.io/yuzjing/tallymind:latest
```

### 🗺️ Roadmap

- [x] OpenAI-compatible multimodal parsing (Text + Receipt Image)
- [x] Beancount plain-text ledger ingestion
- [x] External YAML template hot-reloading
- [x] WeCom / WeChat Work Plugin webhook adapter
- [ ] Automated periodic reporting (Weekly / Monthly digests via Cron)
- [ ] Additional channel adapters (Telegram Bot, Discord, Lark)

---

<a name="chinese"></a>
## 中文

**TallyMind** 是一个基于 Go 语言开发的高性能多模态自动记账服务。支持通过微信或企业微信发送**自然语言、多笔消费短语或账单小票截图**，由多模态大模型智能解析并自动写入本地 [Beancount](https://beancount.github.io/) 纯文本复式记账账本。

### 🚀 核心特性

- **多模态大模型管道**：基于标准 OpenAI Vision 协议（支持 Gemini、DeepSeek、通义千问等），支持**纯文本、多笔拆分、小票截图 OCR 与上下文图文混记**，输出标准化交易数据。
- **Beancount 纯文本复式记账**：数据以标准复式记账语法追加写入本地 `.bean` 文本文件，完美对接 `Fava` 与 `lazybeancount`，零数据库依赖、数据无厂商锁定。
- **动态 YAML/Go 模板渲染引擎**：消息视图层与业务逻辑完全解耦，支持在宿主机直接修改模板实现**秒级热更新**，无需重新构建容器镜像。
- **六边形解耦架构**：严格隔离“消息入站接收”、“核心账本领域模型”与“出站分发渠道”，各组件高内聚、低耦合。
- **微信 / 企微全兼容**：自适应处理企业微信原生交互与个人微信（微工作台）消息排版，避免消息类型兼容性拦截。

### 🏗️ 架构设计

```text
[微信 / 企微 Webhook 入站]
           │
           ▼
[入站回调处理层] (签名校验、AES 解密与 200 OK 快速响应)
           │
           ├─► (异步 Goroutine 协程池)
           ▼
[核心记账处理管道]
   ├──► 1. 多模态 LLM (文本意图识别 / 图像 OCR -> 结构化 JSON)
   ├──► 2. 账本引擎 (字段校验、自动降级与 Beancount 存盘)
   └──► 3. 模板引擎 (YAML / Text 渲染 -> 出站路由分发)
```

### ⚡ 快速部署

#### 1. 配置文件 (`.env`)

参考上方英文说明中的 `.env` 完整示例，配置对应的 LLM 凭证、企微参数与 Beancount 文件路径。

#### 2. 容器启动

```bash
podman run -d \
  --name tallymind \
  --restart always \
  -p 8080:8080 \
  -v ./data:/app/data \
  -v ./templates:/app/templates:ro \
  --env-file .env \
  ghcr.io/yuzjing/tallymind:latest
```

### 🗺️ 开发计划 (Roadmap)

- [x] OpenAI 兼容的多模态解析管道 (文本 + 小票截图)
- [x] Beancount 纯文本复式记账写入与格式化
- [x] 外部挂载式 YAML 模板热更新引擎
- [x] 企业微信 / 微信微工作台 Webhook 适配器
- [ ] 定时财务分析与自动化周报/月报推送 (Cron)
- [ ] 扩展多通道分发支持 (Telegram Bot, 飞书, Discord)

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
```