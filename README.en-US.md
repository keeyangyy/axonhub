<div align="center">

# AxonHub - All-in-one AI Development Platform
### Use any SDK. Access any model. Zero code changes.

</div>

<div align="center">

[![Test Status](https://github.com/keeyangyy/axonhub/actions/workflows/test.yml/badge.svg)](https://github.com/keeyangyy/axonhub/actions/workflows/test.yml)
[![Lint Status](https://github.com/keeyangyy/axonhub/actions/workflows/lint.yml/badge.svg)](https://github.com/keeyangyy/axonhub/actions/workflows/lint.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/keeyangyy/axonhub?logo=go&logoColor=white)](https://golang.org/)

[English](README.en-US.md) | [中文](README.md)

</div>

---


> **This is a downstream branch of [looplj/axonhub](https://github.com/looplj/axonhub)**, tracking upstream and carrying small customisations for our own use.
> Builds here are **unofficial**: versions look like `vX.Y.Z-mine.N` (`N` increments on the fork side), published on [Releases](https://github.com/keeyangyy/axonhub/releases), windows/amd64 only.
> The upstream documentation applies; for the official project see [looplj/axonhub](https://github.com/looplj/axonhub).

## 📖 Project Introduction

### All-in-one AI Development Platform

**AxonHub is the AI gateway that lets you switch between model providers without changing a single line of code.**

Whether you're using OpenAI SDK, Anthropic SDK, or any AI SDK, AxonHub transparently translates your requests to work with any supported model provider. No refactoring, no SDK swaps—just change a configuration and you're done.

**What it solves:**
- 🔒 **Vendor lock-in** - Switch from GPT-4 to Claude or Gemini instantly
- 🔧 **Integration complexity** - One API format for 10+ providers
- 📊 **Observability gap** - Complete request tracing out of the box
- 💸 **Cost control** - Real-time usage tracking and budget management

<div align="center">
  <img src="docs/axonhub-architecture-light.svg" alt="AxonHub Architecture" width="700"/>
</div>

---

### Core Features

| Feature | What You Get |
|---------|-------------|
| 🔄 [**Any SDK → Any Model**](docs/en/api-reference/openai-api.md) | Use OpenAI SDK to call Claude, or Anthropic SDK to call GPT. Zero code changes. |
| 🔍 [**Full Request Tracing**](docs/en/guides/tracing.md) | Complete request timelines with thread-aware observability. Debug faster. |
| 🔐 [**Enterprise RBAC**](docs/en/guides/permissions.md) | Fine-grained access control, usage quotas, and data isolation. |
| ⚡ [**Smart Load Balancing**](docs/en/guides/load-balance.md) | Auto failover in <100ms. Always route to the healthiest channel. |
| 💰 [**Real-time Cost Tracking**](docs/en/guides/cost-tracking.md) | Per-request cost breakdown. Input, output, cache tokens—all tracked. |

---

## 📚 Documentation

For detailed technical documentation, API references, architecture design, and more:

- 📑 **[Documentation Index](docs/en/index.md)** - Complete documentation navigation
## ⭐ Features

### 📸 Screenshots

Here are some screenshots of AxonHub in action:

<table>
  <tr>
    <td align="center">
      <a href="docs/screenshots/axonhub-dashboard.png">
        <img src="docs/screenshots/axonhub-dashboard.png" alt="System Dashboard" width="250"/>
      </a>
      <br/>
      System Dashboard
    </td>
    <td align="center">
      <a href="docs/screenshots/axonhub-channels.png">
        <img src="docs/screenshots/axonhub-channels.png" alt="Channel Management" width="250"/>
      </a>
      <br/>
      Channel Management
    </td>
    <td align="center">
      <a href="docs/screenshots/axonhub-model-price.png">
        <img src="docs/screenshots/axonhub-model-price.png" alt="Model Price" width="250"/>
      </a>
      <br/>
      Model Price
    </td>
  </tr>
  <tr>
  <td align="center">
      <a href="docs/screenshots/axonhub-models.png">
        <img src="docs/screenshots/axonhub-models.png" alt="Models" width="250"/>
      </a>
      <br/>
      Models
    </td>
    <td align="center">
      <a href="docs/screenshots/axonhub-trace.png">
        <img src="docs/screenshots/axonhub-trace.png" alt="Trace Viewer" width="250"/>
      </a>
      <br/>
      Trace Viewer
    </td>
    <td align="center">
      <a href="docs/screenshots/axonhub-requests.png">
        <img src="docs/screenshots/axonhub-requests.png" alt="Request Monitoring" width="250"/>
      </a>
      <br/>
      Request Monitoring
    </td>
  </tr>
</table>

---

### 🚀 API Types

| API Type             | Status     | Description                    | Document                                     |
| -------------------- | ---------- | ------------------------------ | -------------------------------------------- |
| **Text Generation**  | ✅ Done    | Conversational interface       | [OpenAI API](docs/en/api-reference/openai-api.md), [Anthropic API](docs/en/api-reference/anthropic-api.md), [Gemini API](docs/en/api-reference/gemini-api.md) |
| **Image Generation** | ✅ Done | Image generation               | [Image Generation](docs/en/api-reference/image-generation.md) |
| **Rerank**           | ✅ Done    | Results ranking                | [Rerank API](docs/en/api-reference/rerank-api.md) |
| **Embedding**        | ✅ Done    | Vector embedding generation    | [Embedding API](docs/en/api-reference/embedding-api.md) |
| **Realtime**         | 📝 Todo    | Live conversation capabilities | -                                            |

---

### 🤖 Supported Providers

| Provider               | Status     | Supported Models             | Compatible APIs |
| ---------------------- | ---------- | ---------------------------- | --------------- |
| **OpenAI**             | ✅ Done    | GPT-4, GPT-4o, GPT-5, etc.   | OpenAI, Anthropic, Gemini, Embedding, Image Generation |
| **Anthropic**          | ✅ Done    | Claude 3.5, Claude 3.0, etc. | OpenAI, Anthropic, Gemini |
| **Zhipu AI**           | ✅ Done    | GLM-4.5, GLM-4.5-air, etc.   | OpenAI, Anthropic, Gemini |
| **Moonshot AI (Kimi)** | ✅ Done    | kimi-k2, etc.                | OpenAI, Anthropic, Gemini |
| **DeepSeek**           | ✅ Done    | DeepSeek-V3.1, etc.          | OpenAI, Anthropic, Gemini |
| **ByteDance Doubao**   | ✅ Done    | doubao-1.6, etc.             | OpenAI, Anthropic, Gemini, Image Generation |
| **Gemini**             | ✅ Done    | Gemini 2.5, etc.             | OpenAI, Anthropic, Gemini, Image Generation |
| **Fireworks**          | ✅ Done    | MiniMax-M2.5, GLM-5, Kimi K2.5, etc. | OpenAI |
| **Jina AI**            | ✅ Done    | Embeddings, Reranker, etc.   | Jina Embedding, Jina Rerank |
| **OpenRouter**         | ✅ Done    | Various models               | OpenAI, Anthropic, Gemini, Image Generation |
| **ZAI**                | ✅ Done    | -                            | Image Generation |
| **AWS Bedrock**        | 🔄 Testing | Claude on AWS                | OpenAI, Anthropic, Gemini |
| **Google Cloud**       | 🔄 Testing | Claude on GCP                | OpenAI, Anthropic, Gemini |
| **NanoGPT**            | ✅ Done    | Various models, Image Gen    | OpenAI, Anthropic, Gemini, Image Generation |

---

## 🚀 Deployment Guide

This repository only ships windows/amd64 binaries: download the zip from [Releases](https://github.com/keeyangyy/axonhub/releases) and run `axonhub.exe`. For other deployment options and configuration, see the [documentation index](docs/en/index.md) (the upstream documentation applies).

---

## ⚡ Quick Start

After deployment:

1. **Initialize the system**
   - Open the AxonHub URL returned by your agent.
   - Follow the initialization wizard to create the administrator account.
2. **Add a channel**
   - Add an AI provider and its API key under **Channels**.
   - Configure supported models, test the connection, and enable the channel.
3. **Create an API key**
   - Create an AxonHub API key for your client under **API Keys**.
4. **Make your first request**

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

See the [documentation index](docs/en/index.md) for configuration details and more API examples.

---

## 🛠️ Development Guide

For detailed development instructions, architecture design, and contribution guidelines, please see [docs/en/development/development.md](docs/en/development/development.md).

---

## 🤝 Acknowledgments

This project is based on [looplj/axonhub](https://github.com/looplj/axonhub); thanks to its authors and all contributors. Third-party dependencies ([gin](https://github.com/gin-gonic/gin), [ent](https://github.com/ent/ent), [gqlgen](https://github.com/99designs/gqlgen), [shadcn-admin](https://github.com/satnaing/shadcn-admin), [llms](https://github.com/musistudio/llms) and others) remain under their own copyright — see `go.mod` and `package.json` for the full list.

---

## 📄 License

This project is licensed under multiple licenses (Apache-2.0 and LGPL-3.0). See [NOTICE](NOTICE) for the licensing scope, and [LICENSE](LICENSE) and [llm/LICENSE](llm/LICENSE) for the full license texts.

---

<div align="center">

**AxonHub** - All-in-one AI Development Platform, making AI development simpler

[🏠 This repository](https://github.com/keeyangyy/axonhub) • [⬆️ Upstream](https://github.com/looplj/axonhub) • [🐛 Issue Feedback](https://github.com/keeyangyy/axonhub/issues)

Based on [AxonHub](https://github.com/looplj/axonhub) by the AxonHub team

</div>
