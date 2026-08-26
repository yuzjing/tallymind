# TallyMind

TallyMind 是一个基于 Go 语言构建的现代化多模态自动化记账网关与 Beancount 复式记账核心服务。支持企业微信（HTTP / WSS）、标准 REST API 多端接入，内置多大模型服务商自动容灾池，提供时效签名验证的移动端 H5 电子小票与统计大盘（TallyView / Fava）原生反向代理。

---

## 🏛️ 系统架构

TallyMind 遵循整洁架构（Clean Architecture）与端口适配器模式（Ports & Adapters），接入层与底层存储引擎物理隔离，所有记账行为收敛于单核心业务管道：

```text
                        ┌───────────────────────────────┐
                        │      Client / User Inputs     │
                        │  (WeChat / REST API / H5 Web) │
                        └───────────────┬───────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                       Inbound Adapters (传输与协议接入层)                     │
│   • WeCom Plugin (HTTP / WSS)   • REST API (/api/v1)   • Receipt Web Handler│
└───────────────────────────────────────┬─────────────────────────────────────┘
                                        │ (统一通用 DTO)
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Application Service (核心业务应用编排层)                   │
│                          service.AccountingService                          │
│   • Multi-modal Pipeline (AI 识别)       • Direct Ingestion (结构化存盘)     │
│   • Receipt State Cache & URL Signer     • Knowledge Base (成员关系映射)    │
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

## ✨ 核心特性

- 🤖 **多模态智能提取**：支持文本、票据截图、发票 PDF、微信语音等多载体输入，精准解析商户、金额、科目、账户、出资人、受益人及特征标签。
- 🔄 **多 LLM 负载均衡与自动容灾**：支持配置多个大模型提供商（Gemini / DeepSeek / OpenAI 等）与多 API Key，遇到 `429 限流` 或服务异常自动无感故障转移（Failover）。
- 👥 **家庭成员知识库归一化**：通过配置别名字典动态注入 Prompt，自动统一出资人（Owner）与受益人（Beneficiary）标识，开源零隐私泄漏，保证账本统计口径一致。
- 🧾 **时效签名 H5 电子小票**：自动生成 2 小时有效期的安全签名小票，支持在移动端浏览器直观核对交易明细与原始单据快照。
- 📊 **原生统计大盘反代**：内置反向代理网关，无缝直连 Fava / TallyView 容器，零公网暴露即可在小票上一键直达资产与收支图表。
- 📁 **纯文本复式记账**：数据最终以标准 Beancount 格式存储，按年份自动分卷归档（如 `2026.bean`）。

---

## 🚀 快速上手

### 1. 准备配置
复制配置模板并修改你的凭据（详细配置说明见文件内注释）：
```bash
cp config.example.yaml config.yaml
vim config.yaml
```

### 2. 启动运行
- **本地启动**：
  ```bash
  go run cmd/tallymind/main.go
  ```
- **Docker 运行**：
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


