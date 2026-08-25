# 🌿 TallyMind

<p align="left">
  <a href="README.md">English</a> | <b>简体中文</b>
</p>

> 基于 Go 语言开发的高性能多模态智能记账服务。支持发送**自然语言、多笔短语或账单小票截图**，由大模型智能提取并自动写入本地 [Beancount](https://beancount.github.io/) 纯文本复式记账账本。


[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Container Image](https://img.shields.io/badge/GHCR-tallymind-blue?logo=docker)](https://github.com/yuzjing/tallymind/pkgs/container/tallymind)

---

## 🚀 核心特性

- **多模态大模型管道**：基于标准 OpenAI Vision 协议（支持 Gemini、DeepSeek、通义千问等），支持**纯文本、多笔拆分、小票截图 OCR 与上下文图文混记**，输出标准化交易数据。
- **Beancount 纯文本复式记账**：数据以标准复式记账语法追加写入本地 `.bean` 文本文件，完美对接 `Fava` 与 `beancount`，零数据库依赖、数据无厂商锁定。
- **动态 YAML 模板渲染引擎**：消息视图层与业务逻辑彻底解耦，在宿主机直接修改 YAML 模板即可**秒级热更新**，无需重新构建容器镜像。
- **六边形解耦架构**：严格隔离“消息入站接收”、“核心账本领域模型”与“出站分发渠道”，各组件高内聚、低耦合。
- **微信 / 企微全兼容**：自适应处理企业微信与个人微信（微工作台）消息排版差异，避免消息格式兼容性拦截。
- **多 LLM 负载均衡与自动容灾**：支持配置多个大模型服务商或同平台多 Key，遭遇 `429 限流` 或网络波动时秒级自动故障转移（Failover）。
- **统一 YAML 驱动**：使用 `config.yaml` 统一管理应用开关、企微回调、大模型池及 Beancount 账本参数。

---

## 🏗️ 架构设计

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
   └──► 3. 模板引擎 (YAML 渲染 -> 出站路由分发)
```

---

## ⚡ 快速开始

### 1. 配置文件 (`.env`)

参考 [英文版配置说明](README.md#1-configuration)，在部署目录下配置对应的 API 凭证、企微参数与账本文件路径。

### 2. 容器启动

```bash
podman run -d \
  --name tallymind \
  --restart unless-stopped \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/logs:/app/logs \
  ghcr.io/yuzjing/tallymind:latest
```

---

## 🗺️ 开发计划 (Roadmap)

- [x] OpenAI 兼容的多模态解析管道 (文件 + 图像+ 语音)
- [x] Beancount 纯文本复式记账写入与格式化
- [x] 外部挂载式 YAML 模板热更新引擎
- [x] 企业微信 / 微信微工作台 Webhook 适配器
- [ ] 定时财务分析与自动化周报/月报推送 (Cron)
- [ ] 扩展多通道分发支持 (Telegram Bot, 飞书, Discord)

---

## 📄 开源协议

本项目采用 [MIT License](LICENSE) 协议开源。
