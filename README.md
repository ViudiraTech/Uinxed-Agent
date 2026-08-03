# Uinxed Agent

一个运行在终端里的 AI 编程助手（TUI Agent），支持多提供商、工具调用、流式输出与 thinking 展示。设计参考 [opencode](https://opencode.ai)。

![License](https://img.shields.io/badge/License-Apache_2.0-blue)

![PREVIEW](PREVIEW.png)

## 特性

- **多提供商**：内置本地网关与 DeepSeek，`/connect` 可接入任意 OpenAI 兼容服务
- **工具调用**：bash / 读写文件 / 编辑 / 目录 / grep / glob / 网页抓取 / 计算 / 时间，共 10+ 工具
- **多 Agent**：build / plan 主 agent（Tab 切换）+ explorer / general 子 agent（`@name` 委托）
- **流式输出**：SSE 流式渲染，打字机效果
- **Thinking 展示**：推理模型（如 DeepSeek reasoning 系列）的思考过程折叠显示，Enter 展开
- **Markdown 渲染**：标题 / 列表 / 引用 / 代码块 / 行内样式
- **命令面板**：输入 `/` 即时过滤全部命令
- **会话持久化**：历史与配置保存在 `~/.config/ux-agent/`

## 安装

```bash
cd agent
npm install
npm run build
npm link          # 全局安装 ux-agent 命令(可选)
```

## 使用

```bash
ux-agent                          # 启动
ux-agent --provider deepseek      # 指定提供商
ux-agent --key sk-xxx             # 设置 Key
```

### 内嵌命令

| 命令 | 说明 |
|------|------|
| `/provider` | 列出/切换提供商 |
| `/connect` | 接入新的 OpenAI 兼容提供商 |
| `/key` | 设置当前提供商 API Key |
| `/model` | 切换模型 |
| `/thinking` | 开关 thinking 展示 |
| `/agent` | 列出/切换 agent |
| `/quota` | 查询本地网关余额（仅本地提供商） |
| `/cd` `/pwd` | 工作目录 |
| `/new` `/clear` | 会话管理 |
| `/exit` | 退出 |

### 快捷键

- `Tab` — 切换主 agent
- `Enter` — 展开/收起 thinking
- `↑/↓`、`PgUp/PgDn` — 滚动消息
- `/` — 命令面板
- `@agent` — 委托子任务
- `Esc` — 取消弹窗

## 架构

```
src/
├── index.js      # 入口
├── config.js     # 配置与提供商持久化
├── provider.js   # 多提供商适配 / 流式 SSE 解析
├── tools.js      # 工具注册表与执行器
├── agents.js     # 多 Agent 定义与工具白名单
├── App.jsx       # TUI 主界面
├── Markdown.jsx  # Markdown 渲染器
└── Thinking.jsx  # thinking 折叠组件
```

## 许可证

[Apache-2.0](LICENSE)
