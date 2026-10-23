# promptx

提示词上下文——system prompt 分段组装/运行中转向队列/文件读后写一致性状态 —— oagentkit 模块矩阵 **tier 1**（能力层，依赖 tier0）。

```
go get github.com/oagentkit/promptx
```

| 包 | 职责 |
|---|---|
| `sysprompt` | system prompt 分段组装：Section 化收集 → stable/dynamic 分离 |
| `steering` | 运行中转向队列：turn 执行中用户插入的新输入按投递语义生效 |
| `filestate` | 读后写一致性状态：编辑/覆写文件前必须读过，且读后文件未被改动 |

## 定位

**tier 1** — 能力层，依赖 tier0。

- 本仓依赖的矩阵内仓：`runtime`、`textx`。

## 外部依赖地板

- **零第三方依赖。** 只依赖第一方基座 `github.com/oekit/concurrency` 与矩阵内的 `runtime`、`textx`。

## 与 oekit 的关系

本矩阵（oagentkit，Agent/LLM 编排工具链）**依赖** oekit（Open Easy Kit，Go 微服务工具链）：
`oekit/pkg` / `oekit/concurrency` / `oekit/logx` 等。oekit 是另一套独立矩阵，不是子品牌。

## 开发

```bash
go test ./... -race
go vet ./...
gofmt -l .
```

规范见 [AGENTS.md](AGENTS.md)。

## License

MIT
- `sysprompt` 的 Section 化收集必须区分 **stable / dynamic** 两类：
  stable 段进缓存前缀，dynamic 段每轮重算。混淆会导致缓存命中率归零。