# Cursor 助手

维护者：`yhfx186`

Cursor 助手用于将自定义模型 API 接入 Cursor 的本地工作流，让模型渠道、代理策略和运行配置由用户自行管理。

## 主要能力

- 配置 OpenAI 与 Anthropic 兼容模型渠道
- 管理本地代理、后端监听地址和路由模式
- 查看会话、Token 与缓存命中统计
- 支持 Windows、macOS 与 Linux 构建
- 更新信息仅在用户点击“查看上游更新”后通过浏览器查看

## 快速开始

```bash
cd frontend && yarn install --frozen-lockfile && cd ..
task dev
```

构建当前平台分发包：

```bash
task build
```

更多开发和发布说明见 [CONTRIBUTING.md](./CONTRIBUTING.md)。

## 许可证

本项目基于 [MIT License](./LICENSE) 发布。上游版权声明与当前维护者的修改版权声明均保留在许可证文件中。
