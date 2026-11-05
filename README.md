<div align="center">

# AxonHub - All-in-one AI 开发平台
### 任意 SDK、任意模型、零代码改动

</div>

<div align="center">

[![测试状态](https://github.com/keeyangyy/axonhub/actions/workflows/test.yml/badge.svg)](https://github.com/keeyangyy/axonhub/actions/workflows/test.yml)
[![Lint 状态](https://github.com/keeyangyy/axonhub/actions/workflows/lint.yml/badge.svg)](https://github.com/keeyangyy/axonhub/actions/workflows/lint.yml)
[![Go 版本](https://img.shields.io/github/go-mod/go-version/keeyangyy/axonhub?logo=go&logoColor=white)](https://golang.org/)
[![Docker Ready](https://img.shields.io/badge/docker-ready-2496ED?logo=docker&logoColor=white)](https://docker.com)

[English](README.en-US.md) | [中文](README.md)

</div>

---


> **本仓库是 [looplj/axonhub](https://github.com/looplj/axonhub) 的下游分支**，跟随上游合并，并按自用需求做少量定制。
> 这里是**非官方构建**：版本号形如 `vX.Y.Z-mine.N`（`N` 为 fork 侧递增序号），产物见 [Releases](https://github.com/keeyangyy/axonhub/releases)，仅 windows/amd64。
> 上游文档同样适用；官方版本请以 [looplj/axonhub](https://github.com/looplj/axonhub) 为准。

## 📖 项目介绍

### All-in-one AI 开发平台

**AxonHub 是 AI 网关，让你无需改动一行代码即可切换模型供应商。**

无论你使用的是 OpenAI SDK、Anthropic SDK 还是任何 AI SDK，AxonHub 都会透明地将你的请求转换为与任何支持的模型供应商兼容的格式。无需重构，无需更换 SDK——只需更改配置即可。

**它解决了什么问题：**
- 🔒 **供应商锁定** - 从 GPT-4 瞬间切换到 Claude 或 Gemini
- 🔧 **集成复杂性** - 一个 API 格式对接 10+ 供应商
- 📊 **可观测性缺口** - 开箱即用的完整请求追踪
- 💸 **成本控制** - 实时用量追踪和预算管理

<div align="center">
  <img src="docs/axonhub-architecture-light.svg" alt="AxonHub Architecture" width="700"/>
</div>

---

### 核心特性 Core Features

| 特性 | 你能获得什么 |
|------|-------------|
| 🔄 [**任意 SDK → 任意模型**](docs/zh/api-reference/openai-api.md) | 用 OpenAI SDK 调用 Claude，或用 Anthropic SDK 调用 GPT。零代码改动。 |
| 🔍 [**完整请求追踪**](docs/zh/guides/tracing.md) | 线程级可观测性的完整请求时间线。更快定位问题。 |
| 🔐 [**企业级 RBAC**](docs/zh/guides/permissions.md) | 细粒度访问控制、用量配额和数据隔离。 |
| ⚡ [**智能负载均衡**](docs/zh/guides/load-balance.md) | <100ms 自动故障转移。始终路由到最健康的渠道。 |
| 💰 [**实时成本追踪**](docs/zh/guides/cost-tracking.md) | 每次请求的成本明细。输入、输出、缓存 Token——全部追踪。 |

---

## 📚 文档 | Documentation

### 文档索引 | Documentation Index

- 📑 **[完整文档索引](docs/zh/index.md)** - 所有文档导航和快速查找

## ⭐ 特性 | Features

### 📸 截图 | Screenshots

以下是 AxonHub 的实际运行截图：

<table>
  <tr>
    <td align="center">
      <a href="docs/screenshots/axonhub-dashboard.png">
        <img src="docs/screenshots/axonhub-dashboard.png" alt="系统仪表板" width="250"/>
      </a>
      <br/>
      系统仪表板
    </td>
    <td align="center">
      <a href="docs/screenshots/axonhub-channels.png">
        <img src="docs/screenshots/axonhub-channels.png" alt="渠道管理" width="250"/>
      </a>
      <br/>
      渠道管理
    </td>
    <td align="center">
      <a href="docs/screenshots/axonhub-model-price.png">
        <img src="docs/screenshots/axonhub-model-price.png" alt="模型价格" width="250"/>
      </a>
      <br/>
      模型价格
    </td>
  </tr>
  <tr>
   <td align="center">
      <a href="docs/screenshots/axonhub-models.png">
        <img src="docs/screenshots/axonhub-models.png" alt="模型" width="250"/>
      </a>
      <br/>
      模型
    </td>
    <td align="center">
      <a href="docs/screenshots/axonhub-trace.png">
        <img src="docs/screenshots/axonhub-trace.png" alt="追踪查看" width="250"/>
      </a>
      <br/>
      追踪查看
    </td>
    <td align="center">
      <a href="docs/screenshots/axonhub-requests.png">
        <img src="docs/screenshots/axonhub-requests.png" alt="请求监控" width="250"/>
      </a>
      <br/>
      请求监控
    </td>

  </tr>
</table>

---

### 🚀 API 类型 | API Types

| API 类型 | 状态 | 描述 | 文档 |
|---------|--------|-------------|--------|
| **文本生成（Text Generation）** | ✅ Done | 对话交互接口 | [OpenAI API](docs/zh/api-reference/openai-api.md)、[Anthropic API](docs/zh/api-reference/anthropic-api.md)、[Gemini API](docs/zh/api-reference/gemini-api.md) |
| **图片生成（Image Generation）** | ✅ Done | 图片生成 | [Image Generation](docs/zh/api-reference/image-generation.md) |
| **重排序（Rerank）** | ✅ Done | 结果排序 | [Rerank API](docs/zh/api-reference/rerank-api.md) |
| **嵌入（Embedding）** | ✅ Done | 向量嵌入生成 | [Embedding API](docs/zh/api-reference/embedding-api.md) |
| **实时对话（Realtime）** | 📝 Todo | 实时对话功能 | - |

---

### 🤖 支持的提供商 | Supported Providers

| 提供商 Provider        | 状态 Status | 支持模型 Models              | 兼容 API |
| ---------------------- | ---------- | ---------------------------- | --------------- |
| **OpenAI**             | ✅ 已完成   | GPT-4, GPT-4o, GPT-5 等      | OpenAI, Anthropic, Gemini, Embedding, Image Generation |
| **Anthropic**          | ✅ 已完成   | Claude 3.5, Claude 3.0 等    | OpenAI, Anthropic, Gemini |
| **智谱 AI (Zhipu)**    | ✅ 已完成   | GLM-4.5, GLM-4.5-air 等      | OpenAI, Anthropic, Gemini |
| **月之暗面 (Moonshot)** | ✅ 已完成   | kimi-k2 等                   | OpenAI, Anthropic, Gemini |
| **DeepSeek**           | ✅ 已完成   | DeepSeek-V3.1 等             | OpenAI, Anthropic, Gemini |
| **字节跳动豆包**        | ✅ 已完成   | doubao-1.6 等                | OpenAI, Anthropic, Gemini, Image Generation |
| **Gemini**             | ✅ 已完成   | Gemini 2.5 等                | OpenAI, Anthropic, Gemini, Image Generation |
| **Jina AI**            | ✅ 已完成   | Embeddings, Reranker 等      | Jina Embedding, Jina Rerank |
| **OpenRouter**         | ✅ 已完成   | 多种模型                     | OpenAI, Anthropic, Gemini, Image Generation |
| **ZAI**                | ✅ 已完成   | -                            | Image Generation |
| **AWS Bedrock**        | 🔄 测试中  | Claude on AWS                | OpenAI, Anthropic, Gemini |
| **Google Cloud**       | 🔄 测试中  | Claude on GCP                | OpenAI, Anthropic, Gemini |
| **NanoGPT**            | ✅ 已完成  | 多种模型、图像生成             | OpenAI, Anthropic, Gemini, Image Generation |

---


## 🚀 部署指南 | Deployment Guide

部署方式（二进制、Docker Compose、Kubernetes）见 [完整文档索引](docs/zh/index.md) 中的部署章节。

---

## ⚡ 快速开始 | Quick Start

部署完成后：

1. **初始化系统**
   - 打开 Agent 返回的 AxonHub 访问地址。
   - 按照初始化向导创建管理员账号。
2. **添加渠道**
   - 在 **Channels（渠道）** 中添加 AI 提供商及其 API Key。
   - 配置支持的模型，测试连接并启用渠道。
3. **创建 API Key**
   - 在 **API Keys（API 密钥）** 中创建供客户端使用的 AxonHub API Key。
4. **发起第一个请求**

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8090/v1",
    api_key="your-axonhub-api-key"
)

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello, AxonHub!"}]
)

print(response.choices[0].message.content)
```

更多配置和 API 示例请查看[完整文档索引](docs/zh/index.md)。

---

## 🛠️ 开发指南

详细的开发说明、架构设计和贡献指南，请查看 [docs/zh/development/development.md](docs/zh/development/development.md)。

---

## 🤝 致谢 | Acknowledgments

本项目基于 [looplj/axonhub](https://github.com/looplj/axonhub)，感谢上游作者与所有贡献者。第三方依赖（[gin](https://github.com/gin-gonic/gin)、[ent](https://github.com/ent/ent)、[gqlgen](https://github.com/99designs/gqlgen)、[shadcn-admin](https://github.com/satnaing/shadcn-admin)、[llms](https://github.com/musistudio/llms) 等）版权归各自作者所有，完整清单见 `go.mod` 与 `package.json`。

---

## 📄 许可证 | License

本项目采用多种许可证授权（Apache-2.0 和 LGPL-3.0）。授权范围详见 [NOTICE](NOTICE)，协议全文见 [LICENSE](LICENSE) 和 [llm/LICENSE](llm/LICENSE)。

---

<div align="center">

**AxonHub** - All-in-one AI 开发平台，让 AI 开发更简单

[🏠 本仓库](https://github.com/keeyangyy/axonhub) • [⬆️ 上游项目](https://github.com/looplj/axonhub) • [🐛 问题反馈](https://github.com/keeyangyy/axonhub/issues)

Based on [AxonHub](https://github.com/looplj/axonhub) by the AxonHub team

</div>
