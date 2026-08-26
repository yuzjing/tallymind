# TallyMind

TallyMind is a modern, multimodal automated bookkeeping gateway and Beancount double-entry accounting core service built with Go. It supports Enterprise WeChat (WeCom HTTP / WSS) and standard REST API ingestion, features multi-LLM automated failover pools, provides time-limited cryptographically signed mobile H5 receipts, and integrates dashboard (TallyView / Fava) reverse proxying.

---

## 🏛️ System Architecture

TallyMind follows a strict layered Clean Architecture (Ports & Adapters). Transport adapters are completely decoupled from the underlying storage engine, and all bookkeeping logic converges into a single application service pipeline:

```text
                        ┌───────────────────────────────┐
                        │      Client / User Inputs     │
                        │  (WeChat / REST API / H5 Web) │
                        └───────────────┬───────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                       Inbound Adapters (Transport Layer)                    │
│   • WeCom Plugin (HTTP / WSS)   • REST API (/api/v1)   • Receipt Web Handler│
└───────────────────────────────────────┬─────────────────────────────────────┘
                                        │ (Normalized DTO)
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                 Application Service (Business Orchestration Layer)          │
│                          service.AccountingService                          │
│   • Multi-modal Pipeline (AI Parsing)    • Direct Ingestion (Structured)    │
│   • Receipt State Cache & URL Signer     • Knowledge Base (Member Mapping)  │
└──────────────────────────┬─────────────────────────────┬────────────────────┘
                           │                             │
            ┌──────────────┴──────────────┐              │
            ▼                             ▼              ▼
┌─────────────────────────┐   ┌─────────────────────────┐   ┌─────────────────┐
│      internal/llm       │   │     internal/ledger     │   │ internal/crypto │
│ • Provider Failover Pool│   │ • Beancount Engine      │   │ • Token Signer  │
│ • Multi-modal Parser    │   │ • Yearly Split & Storage│   │ • HMAC Verifier │
└─────────────────────────┘   └───────────┬─────────────┘   └─────────────────┘
                                          │
                                          ▼
                             [ Data: YYYY.bean Storage ]
```

---

## ✨ Key Features

- 🤖 **Multimodal AI Bookkeeping**: Parses natural language text, receipt snapshots, PDF invoices, and voice audio into structured Beancount entries (amount, payee, category, account, owner, beneficiary, tags).
- 🔄 **Multi-LLM Load Balancing & Failover**: Automatic fallback across multiple AI providers (Gemini / DeepSeek / OpenAI) and API keys on rate limits (HTTP 429) or network errors.
- 👥 **Family Member Knowledge Mapping**: Injects alias knowledge base from `config.yaml` to normalize `owner` and `beneficiary` identities while keeping open-source configuration privacy-safe.
- 🧾 **Signed Mobile H5 Receipts**: Generates time-limited (2-hour TTL) signed receipt URLs for mobile verification of transaction breakdowns and snapshot attachments.
- 📊 **Secure Dashboard Reverse Proxy**: Built-in reverse proxy gateway to seamlessly embed Fava / TallyView containers with zero public port exposure.
- 📁 **Plain-Text Accounting**: Appends verified entries into standard Beancount ledgers with automatic yearly file splitting (`2026.bean`).

---

## 🚀 Quick Start

### 1. Configuration
Copy the configuration template and fill in your credentials (see comments inside `config.example.yaml` for details):
```bash
cp config.example.yaml config.yaml
vim config.yaml
```

### 2. Run
- **Locally**:
  ```bash
  go run cmd/tallymind/main.go
  ```
- **With Docker**:
  ```bash
  docker run -d \
    --name tallymind \
    -p 8080:8080 \
    -v $(pwd)/config.yaml:/app/config.yaml:ro \
    -v $(pwd)/data:/app/data \
    -v $(pwd)/logs:/app/logs \
    ghcr.io/yuzjing/tallymind:latest
  ```

---

## 📄 License

MIT License © [yuzjing](https://github.com/yuzjing)
