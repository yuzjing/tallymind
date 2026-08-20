# 🌿 TallyMind

<p align="left">
  <b>English</b> | <a href="README_zh.md">简体中文</a>
</p>

> An automated, multimodal double-entry bookkeeping agent written in Go. It turns natural language chat and receipt screenshots into strict [Beancount](https://beancount.github.io/) plain-text records.


[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Container Image](https://img.shields.io/badge/GHCR-tallymind-blue?logo=docker)](https://github.com/yuzjing/tallymind/pkgs/container/tallymind)

---

## 🚀 Key Features

- **Multimodal AI Pipeline**: Extracts multi-transaction records and receipt screenshots into structured JSON via an OpenAI-compatible Vision API (e.g., Gemini, DeepSeek, Qwen-VL).
- **Plain-Text Accounting (PTA)**: Appends standardized entries directly to local `.bean` files, fully compatible with `Fava` and `beancount` without vendor lock-in.
- **Dynamic YAML Template Engine**: Decouples UI presentation from backend code; templates support runtime hot-reloading on the host without container restarts.
- **Hexagonal Architecture**: Isolates the core accounting domain from inbound webhooks and outbound messaging adapters.
- **Dual-Client Compatibility**: Automatically formats messages for both Enterprise WeChat and Personal WeChat (via WeChat Work Plugin).

---

## 🏗️ Architecture

```text
[WeChat / WeCom Webhook]
           │
           ▼
[Inbound Callback Handler] (Signature Verification & AES Decryption)
           │
           ├─► (Async Worker Goroutine)
           ▼
[Core Transaction Pipeline]
   ├──► 1. Multimodal LLM (Text Intent / Image OCR -> Structured JSON)
   ├──► 2. Ledger Engine (Field Validation & Beancount Append)
   └──► 3. Template Engine (YAML Rendering -> Outbound Dispatch)
```

---

## ⚡ Quick Start

### 1. Configuration (`.env`)

```env
# Server
APP_ENV=production
LOG_LEVEL=info
SERVER_PORT=8080
PUBLIC_URL=your_public_url
TEMPLATE_DIR=templates

# Features
ENABLE_HTTP_API=true
ENABLE_LLM=true
ENABLE_WECOM_HTTP=true
ENABLE_REPORTER=false

# LLM (OpenAI-Compatible)
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

# Ledger Storage
BEANCOUNT_FILE_PATH=data/2026.bean
DEFAULT_CURRENCY=CNY
DEFAULT_REPORTER=User
FALLBACK_CATEGORY=Expenses:Uncategorized
FALLBACK_ACCOUNT=Assets:Pending:Unknown
```

### 2. Run Container

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

---

## 🗺️ Roadmap

- [x] OpenAI-compatible multimodal parsing (File + Image + Voice)
- [x] Beancount plain-text ledger ingestion
- [x] External YAML template hot-reloading
- [x] WeCom / WeChat Work Plugin webhook adapter
- [ ] Automated periodic reporting (Weekly / Monthly digests via Cron)
- [ ] Multi-channel adapters (Telegram Bot, Lark, Discord)

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
