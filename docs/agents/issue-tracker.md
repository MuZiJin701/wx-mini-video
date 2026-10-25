# Issue tracker：GitHub

需求、缺陷和规格记录在 GitHub Issues：`MuZiJin701/wx-mini-video`。仓库目录内用 `gh` 由 remote 推断仓库；其他目录加 `--repo MuZiJin701/wx-mini-video`。

## 操作

- 创建：`gh issue create --title "..." --body-file <正文文件>`
- 查看：`gh issue view <number> --json number,title,body,labels,comments`
- 列表：`gh issue list --state open --json number,title,body,labels,comments`，可加 `--label`、`--state`；完整队列用分页 API
- 评论/标签/关闭：`gh issue comment`、`gh issue edit --add-label/--remove-label`、`gh issue close --comment`
- 子 issue：`gh issue create --parent <parent> ...` 或 `gh issue edit <parent> --add-sub-issue <child>`
- 阻塞：`gh issue edit <child> --add-blocked-by <blocker>`
- 认领：`gh issue edit <number> --add-assignee "@me"`

多行正文先写 UTF-8 文件再用 `--body-file`；子 issue 命令需要 `gh` 2.94+，其他命令以本机 `--help` 为准。技能要求“发布到 issue tracker”时创建 issue，要求获取 ticket 时按上面“查看”读取正文、标签与评论。

## Pull request

外部 PR 不进入自动 triage 队列；用户明确指定的 PR 仍按请求处理。issue 与 PR 共用编号，无法判断时先用 `gh pr view <number>` 排除 PR。

## Wayfinding

供已安装的 `/wayfinder` 使用：地图 issue 用 `wayfinder:map` 标签，ticket 用 `wayfinder:research|prototype|grilling|task` 并关联为子 issue（不支持子 issue 时在正文首行写 `Part of #<map>`）。阻塞用 GitHub 原生 dependencies（不支持时正文首行写 `Blocked by: #<number>`）；按地图顺序选未关闭、无开放阻塞且无人认领的第一个 ticket。完成后先评论结论、关闭 ticket，并在地图 Decisions-so-far 补简短结论与链接。`wayfinder:*` 标签按需创建。
