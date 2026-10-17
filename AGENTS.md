# AGENTS.md — oagentkit/promptx

> 提示词上下文——system prompt 分段组装/运行中转向队列/文件读后写一致性状态
> module：`github.com/oagentkit/promptx` · org：[oagentkit = Open Agent Kit](https://github.com/oagentkit)
> 矩阵分层：**tier 1**（能力层，依赖 tier0）

## 本仓包含

```
`sysprompt/`
`steering/`
`filestate/`
```

## 硬规则

### 分层与依赖

- 依赖严格自底向上，**禁止反向依赖**。本仓是 tier 1，只允许依赖 tier ≤ 1 的仓。
- 本仓依赖的矩阵内仓：`runtime`、`textx`。
- 改 module 路径必须同步：全部 import、go.mod、README 安装行。

### 错误处理

- 一律使用 `github.com/oekit/pkg/errors`；业务错误用 BizError。
- 所有 error 禁止 `_` 忽略。

### 并发安全

- 禁止裸 `go func`：用 `github.com/oekit/concurrency/async` 的 GoSafe 系列（panic 被 recover）。
- 共享状态用 mutex / atomic 保护；包级全局可变状态用 `atomic.Pointer[T]`。

### API 设计

- 函数选项模式命名 `XxxOption`（禁止裸 `Option`，多包 co-import 会冲突）。
- 导出符号 PascalCase + godoc 注释；包文档放 `doc.go`。

### 日志

- **本仓不引入日志依赖。** tier0 加日志等于给所有下游抬地板；需要观测输出时返回 error 或交给调用方。

## 仓特有纪律

- `sysprompt` 的 Section 化收集必须区分 **stable / dynamic** 两类：
  stable 段进缓存前缀，dynamic 段每轮重算。混淆会导致缓存命中率归零。

## 明确不属于本仓的

| 能力 | 归属 |
|---|---|
| 文本/差异/会话/报告 | `github.com/oagentkit/textx` |
| 事件总线/流控/子进程/热插拔/钩子 | `github.com/oagentkit/runtime` |
| 脱敏/出口围栏/权限/沙箱解析/审计 | `github.com/oagentkit/guardx` |
| DB 队列与选主 | `github.com/oagentkit/queue` |
| 通用 errors/stringx/encoding/mathx | `github.com/oekit/pkg` |
| goroutine 池/单飞/重试/令牌桶 | `github.com/oekit/concurrency` |
