# promptx CHANGELOG

> 本文件由 `matrix-tools/scripts/gen_changelogs.py` 从 git 历史生成，按 tag 切段、取每段内的 conventional commit 渲染。

## [v0.8.0] — 2026-10-19

- 杂务(deps)：依赖地板对齐远端最新（runtime v0.7.0 / textx v0.5.0） (13)

## [v0.7.0] — 2026-10-19

- 杂务(deps)：依赖地板对齐远端最新（runtime v0.6.0 / textx v0.4.0 / concurrency v0.17.1 / pkg v0.15.1） (#13)

## [v0.6.0] — 2026-10-19

- 杂务(deps)：oekit 依赖升级（concurrency v0.16.0→v0.17.0）

## [v0.5.1] — 2026-10-18

- 与 v0.5.0 相比无代码差异（版本号/打包变更）

## [v0.5.0] — 2026-10-18

### 新增：promptx 仓建立——提示词上下文（3 包）

从 agentkit 单仓切出 tier1 能力层模块，module 路径 github.com/oagentkit/promptx。
收编 3 个包：sysprompt / steering / filestate
矩阵内依赖：runtime（procx）、textx（textutil），跨仓依赖从已发布 tag 解析。
门禁：go build / go vet / go test -race 3 包全绿，gofmt 干净。

### 修复(filestate)：TestPathNormalization 大小写断言改为平台相关

normalizePath 只在 darwin/windows 折叠大小写（大小写不敏感文件系统）；
Linux 上 a.go 与 A.GO 本就是两个不同文件，不折叠是正确行为。
原测试无条件断言 A.GO 命中同一状态，只在 macOS 上能过 —— CI 首次在
Linux 上跑就暴露了这个问题（拆分前本仓没有 CI）。
断言改为与 normalizePath 的平台判定保持一致。
同一处修复已回填原 agentkit 单仓。

- 杂务(deps)：依赖地板升到「历史最新 + 0.1.0」 (#4)
- 杂务(deps)：跟进最新 tag (#3)
- 杂务(deps)：oekit 依赖升级 (#2)
- 杂务(deps)：跨仓依赖升级 (#1)
- 杂务(deps)：oekit/concurrency v0.12.0 -> v0.13.1
- 杂务(deps)：oekit 三模块升至最新（pkg v0.13.0 / concurrency v0.12.0 / logx v0.7.0）
- 杂务(deps)：oekit/concurrency v0.6.0 -> v0.9.0

- CI：接入 org 共享门禁（规则本体在 oagentkit/.github）
