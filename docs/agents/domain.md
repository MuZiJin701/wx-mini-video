# Domain docs

## 读取规则

探索代码前，按主题读取：

- 根目录 `GLOSSARY.md`：本仓库领域术语的唯一来源。
- `docs/adr/` 中涉及当前区域的决策记录。
- 需要实现、配置或运行细节时读取 `wx-mini-video-knowledge.md`。

文件不存在时直接继续；有明确的术语或决策需要记录时再创建。

在 issue 标题、设计建议、假设、测试名称和代码标识中使用术语表定义的名称，避免其 `_Avoid_` 中的同义词。概念不在术语表时，先判断是否误用了领域语言；确有缺口时通过 `/domain-modeling` 澄清并记录。

若提议与已有 ADR 冲突，指出具体 ADR、冲突内容及重新讨论的理由。

## 布局与维护

这是单上下文仓库：

```text
/
├── GLOSSARY.md
├── docs/adr/
│   ├── 0001-managed-ffmpeg.md
│   ├── 0002-portable-runtime-data.md
│   ├── 0003-target-appid-filtering.md
│   ├── 0004-windows-terminal-startup.md
│   ├── 0005-local-ca-lifecycle.md
│   ├── 0006-macos-portable-distribution.md
│   ├── 0007-bundled-ffmpeg.md
│   ├── 0008-target-attribution.md
│   └── 0009-user-scoped-windows-runtime.md
└── wx-mini-video-knowledge.md
```

`GLOSSARY.md` 只保存领域术语；实现细节、配置、规格和临时计划写入相应文档。术语明确后即时更新定义。

只有在决策难以逆转、缺少背景会令人意外且经过真实取舍时，才创建 `docs/adr/NNNN-slug.md`。编号接续现有 ADR；已有决策变化时明确记录替代关系。
