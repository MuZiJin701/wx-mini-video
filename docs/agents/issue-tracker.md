# Issue tracker

本仓库的需求、缺陷和规格记录在 GitHub Issues：`MuZiJin701/wx-mini-video`。在仓库目录内使用 `gh`，由 Git remote 推断目标仓库。

## 操作

- 创建：`gh issue create --title "..." --body "..."`。
- 查看：`gh issue view <number> --comments`。
- 列表：`gh issue list --state open`，按需加 `--label`。
- 评论：`gh issue comment <number> --body "..."`。
- 标签：`gh issue edit <number> --add-label "..."` 或 `--remove-label "..."`。
- 关闭：`gh issue close <number> --comment "..."`。

当工程技能要求“发布到 issue tracker”时创建 GitHub issue；要求“获取相关 ticket”时运行 `gh issue view <number> --comments`。

## Pull request

**PRs as a request surface: no.** 外部 Pull Request 不进入 triage 队列。
