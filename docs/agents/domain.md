# Domain docs

## 使用方式

开始探索代码前，按主题读取：

- 根目录 `CONTEXT.md`；
- 涉及区域的 `docs/adr/` 决策记录；
- 需要实现、配置或运行细节时的 `wx-mini-video-knowledge.md`。

文件不存在时直接继续。术语采用 `CONTEXT.md` 的定义；若改动与已有 ADR 冲突，明确指出冲突而不静默覆盖。

## 布局

这是单上下文仓库：根目录 `CONTEXT.md` 只保存术语。只有在决策难以逆转、缺少背景会令人意外且经过真实取舍时，才创建 `docs/adr/NNNN-slug.md`。
