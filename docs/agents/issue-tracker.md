# Issue tracker：GitHub

本仓库的需求、缺陷和规格记录在 GitHub Issues：`MuZiJin701/wx-mini-video`。在仓库目录内使用 `gh`，由 Git remote 推断目标仓库；在其他目录运行时显式传入 `--repo MuZiJin701/wx-mini-video`。

## 操作

- 创建：`gh issue create --title "..." --body-file <正文文件>`。
- 查看：`gh issue view <number> --json number,title,body,labels,comments`。
- 列表：`gh issue list --state open --json number,title,body,labels,comments`，按需加 `--label` 和 `--state`；结果可能受默认条数限制，完整队列使用分页 API。
- 评论：`gh issue comment <number> --body-file <正文文件>`。
- 标签：`gh issue edit <number> --add-label "..."` 或 `--remove-label "..."`。
- 关闭：`gh issue close <number> --comment "..."`。
- 子 issue：`gh issue create --parent <parent> ...`，或 `gh issue edit <parent> --add-sub-issue <child>`。
- 阻塞关系：`gh issue edit <child> --add-blocked-by <blocker>`。

多行正文先写入 UTF-8 文件，再用 `--body-file` 传入。子 issue 命令需要 `gh` 2.94+；其他命令以本机 `gh issue <操作> --help` 为准。旧版 CLI 可使用 GitHub API；仅在仓库不支持对应能力时，改用下述正文关联。

当工程技能要求“发布到 issue tracker”时创建 GitHub issue；要求“获取相关 ticket”时按“查看”读取正文、标签与评论。

## Pull request

**PRs as a request surface: no.** 外部 Pull Request 不进入自动发现的 triage 队列；用户明确指定的 PR 仍按该请求处理。

GitHub 的 issue 与 PR 共用编号；遇到无法判断类型的 `#<number>`，先用 `gh pr view <number>` 判断，确认不是 PR 后再用 `gh issue view <number>`。认证、网络或权限失败不作为“不是 PR”的依据。

## Wayfinding

供已安装的 `/wayfinder` 使用；地图是一个 issue，ticket 是关联到地图的子 issue。

- 地图：使用 `wayfinder:map` 标签，正文维护 Notes、Decisions-so-far 与 Fog。
- Ticket：使用 `wayfinder:research`、`wayfinder:prototype`、`wayfinder:grilling` 或 `wayfinder:task` 标签，关联为地图的子 issue。不支持子 issue 时，在 ticket 正文首行写 `Part of #<map>`，并在地图正文维护任务列表。
- 阻塞：使用 GitHub 原生 issue dependencies。不支持时，在正文首行写 `Blocked by: #<number>, ...`。所有阻塞项关闭后才能推进。
- 前沿：按地图顺序选取尚未关闭、没有开放阻塞项且无人认领的第一个 ticket。读取地图关联的完整队列；检查原生关系的 `issue_dependencies_summary.blocked_by`，或读取正文关联的阻塞 issue 状态。
- 认领：`gh issue edit <number> --add-assignee "@me"`。
- 完成：先评论结论，再关闭 ticket，并在地图的 Decisions-so-far 补上简短结论和链接。

`wayfinder:*` 标签在实际使用 wayfinder 时按需创建。
