// Package memorypipe 记忆的**注入-引用-打标**三段（读写时机归宿主）。
//
// 本包自 goagent-kernel 平移而来（归属评测 2026-11-26：对话域，判据主语不是编排）。
// API 承诺：0.x 阶段 minor 允许 breaking（org 通行约定）。
//
// # 为什么只做这三段
//
// 「什么时候写记忆、写什么、摘要怎么生成」全是宿主的上下文策略——本仓
// @@budget@@ 已经把摘要生成留成 @@Summarizer@@ 接口，同一条边界在记忆上成立：
// 内核不该替宿主决定「这次对话值不值得记住」。
//
// 但**注入**那一侧有三件事与策略无关、且各宿主各写一遍必然各判一套：
//
//  1. **来源**：一条没有来源的记忆与模型的猜测在文本上无法分辨。所以来源
//     不是可选的元数据，是**注入的前提**——没有来源的条目不注入，而是显式
//     丢弃并给出原因。
//  2. **打标**：注入体必须逐条带 id / source / at 标记，模型才能引用它
//     （而引用是「这条依据从哪来」的唯一可追溯路径），宿主才能据此判新鲜度。
//  3. **裁剪显式**：预算裁掉了哪几条必须说出来。静默截断会让模型以为
//     「记忆里只有这些」，而实际是「更多被挤掉了」。
//
// # scope 精确匹配，不做父级兜底
//
// 按「project」查却回退到「global」看起来更友好，代价是**作用域失去了意义**：
// 一条只该在某个项目里生效的记忆会在任何会话里生效，而报告完全正常。
// 故 scope 不匹配就是丢弃，原因写清。
//
// 生命周期: Experimental——未过一个 minor 无 breaking 窗口（api-compat.md 晋级条件一），初值不承诺兼容
package memorypipe

import (
	"slices"
	"strings"
	"time"

	"github.com/oekit/pkg/errors"
	"github.com/oekit/pkg/stringx"
)

// Entry 一条记忆（值类型）。
//
// ID 与 Source 都由宿主填：本包不生成 ID（那是存储的活），也不猜来源。
type Entry struct {
	// ID 稳定标识（渲染时进标记，模型据此引用）。
	ID string
	// Text 正文。
	Text string
	// Scope 作用域（宿主自定词表；本包只做**精确匹配**）。
	Scope string
	// Tags 标签（查询按 AND 匹配）。
	Tags []string
	// Source 来源（**必填**：没有来源的条目与猜测不可分）。
	Source string
	// At 产生时刻（渲染进标记，供模型判新鲜度）。
	At time.Time
}

// Query 注入请求。
type Query struct {
	// Scope 只要这个作用域的条目。
	Scope string
	// Tags 必须全部命中（AND；OR 会让「加个标签」变成放宽而不是收紧）。
	Tags []string
	// MaxItems 最多注入几条（<=0 = 不限制）。
	MaxItems int
	// MaxBytes 正文总字节上限（<=0 = 不限制）。
	MaxBytes int
}

// Drop 一条被丢弃的条目及其原因。
//
// 丢弃必须**可解释**：宿主据此判断「是配置问题还是预算问题」——
// 两者在「记忆怎么没出现」这个现象上完全一样。
type Drop struct {
	ID   string
	Why  string
	Text string // 摘要（截断），便于宿主排查
}

// Piece 一条实际注入的片段。
type Piece struct {
	Entry Entry
	Text  string // 可能被字节裁剪过的正文
}

// Result 一次注入的结果。
type Result struct {
	Pieces    []Piece
	Dropped   []Drop
	Truncated bool
	Reason    string
}

// Build 按查询挑出要注入的记忆条目。
func Build(entries []Entry, q Query) (Result, error) {
	var res Result
	if strings.TrimSpace(q.Scope) == "" {
		return res, errors.New("memorypipe: 查询没有 scope（作用域不明的注入无从判定该不该生效）")
	}

	// 同 ID 或同正文只留**最新**一条：重复注入既挤占窗口，又让「哪条是当前的」
	// 无从判断（两条正文不同而 ID 相同的记忆，读者只能猜）。
	latest := map[string]Entry{}
	for _, e := range entries {
		key := e.ID
		if key == "" {
			key = "text:" + e.Text
		}
		prev, ok := latest[key]
		if !ok || e.At.After(prev.At) {
			latest[key] = e
		}
	}

	keys := make([]string, 0, len(latest))
	for k := range latest {
		keys = append(keys, k)
	}
	// 确定性：按 (At 降序, 去重键升序)。时间相同也不能靠 map 顺序决定先后；
	// 回退比较用**去重键**而不是条目 ID（issue #203）：空 ID 是合法输入
	// （ineligible 只拒「ID 与 Text 同时为空」），两条空 ID 条目按 ID 比较恒等，
	// slices.SortFunc 非稳定——输出顺序随 map 迭代序漂移，配 MaxItems/MaxBytes
	// 时哪条被注入随运行变化，Result 不可复现。去重键对空 ID 条目是
	// "text:"+Text，必不相同（同键已在去重时合并）。
	slices.SortFunc(keys, func(a, b string) int {
		ea, eb := latest[a], latest[b]
		if c := ea.At.Compare(eb.At); c != 0 {
			return -c
		}
		return strings.Compare(a, b)
	})

	usedBytes := 0
	for _, k := range keys {
		e := latest[k]
		if why := ineligible(e, q); why != "" {
			res.Dropped = append(res.Dropped, Drop{ID: e.ID, Why: why, Text: clip(e.Text, 60)})
			continue
		}
		if q.MaxItems > 0 && len(res.Pieces) >= q.MaxItems {
			res.Truncated = true
			res.Dropped = append(res.Dropped, Drop{ID: e.ID, Why: "超过 MaxItems"})
			continue
		}
		text := e.Text
		if q.MaxBytes > 0 {
			if usedBytes+len(text) > q.MaxBytes {
				remain := q.MaxBytes - usedBytes
				if remain <= 0 {
					res.Truncated = true
					res.Dropped = append(res.Dropped, Drop{ID: e.ID, Why: "超过 MaxBytes"})
					// Reason 必须显式赋值（issue #201）：这里不赋值会被循环末尾
					// 的兜底改写成「候选数超过 MaxItems」——字节预算耗尽被报成
					// 条数预算耗尽，宿主调 MaxItems 永远修不好，排障方向直接错。
					res.Reason = "预算耗尽（超过 MaxBytes），其余条目未注入"
					continue
				}
				text = clip(text, remain)
				res.Truncated = true
				res.Reason = "正文超过 MaxBytes，已逐条裁剪"
			}
			usedBytes += len(text)
		}
		// 值类型承诺（issue #394）：Entry 自称「值类型」，但 Tags 是切片。
		// 按值放进 Piece 只复制切片头，底层数组仍与调用方入参共享——改标签
		// 等于改 AND 匹配的判据，两边的匹配结果随之变。
		e.Tags = slices.Clone(e.Tags)
		res.Pieces = append(res.Pieces, Piece{Entry: e, Text: text})
	}
	if len(res.Pieces) == 0 && len(res.Dropped) == 0 {
		res.Reason = "范围内没有任何记忆条目"
	}
	if res.Truncated && res.Reason == "" {
		res.Reason = "候选数超过 MaxItems"
	}
	return res, nil
}

// ineligible 判一条记忆为什么不适合注入（空 = 可以注入）。
func ineligible(e Entry, q Query) string {
	if strings.TrimSpace(e.ID) == "" && strings.TrimSpace(e.Text) == "" {
		return "既没有 ID 也没有正文"
	}
	if strings.TrimSpace(e.Source) == "" {
		return "没有来源（没有来源的记忆与猜测在文本上不可分）"
	}
	if e.Scope != q.Scope {
		return "作用域不匹配（" + e.Scope + "）；不做父级兜底，否则作用域失去意义"
	}
	for _, want := range q.Tags {
		if !slices.Contains(e.Tags, want) {
			return "缺少标签 " + want
		}
	}
	return ""
}

// Prompt 渲染成可注入的文本：**逐条打标**。
//
// 标记里带 id / source / at 三样，缺一样就少一条追溯路径：
// 没有 id 无法引用，没有 source 无法判断可信度，没有 at 无法判新鲜度。
// ID 为空的条目（合法输入）显式标注「无 id」（issue #203）——渲染出一个
// 空的 id= 槽位会让模型拿一个空串当引用锚点，且看不出是「这条没有 id」
// 还是「标记坏了」。
func (r Result) Prompt() string {
	if len(r.Pieces) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<memory>\n")
	for _, p := range r.Pieces {
		b.WriteString("[id=")
		if p.Entry.ID == "" {
			b.WriteString("<无id>")
		} else {
			b.WriteString(p.Entry.ID)
		}
		b.WriteString(" source=")
		b.WriteString(p.Entry.Source)
		b.WriteString(" at=")
		b.WriteString(p.Entry.At.UTC().Format(time.RFC3339))
		b.WriteString("]\n")
		b.WriteString(p.Text)
		b.WriteString("\n")
	}
	if r.Truncated {
		b.WriteString("[已按预算裁剪：部分条目未注入]\n")
	}
	b.WriteString("</memory>")
	return b.String()
}

// clip 把正文裁到 n 字节以内（不足则原样返回），省略号**计入预算**。
//
// issue #201：旧实现 TruncBytes(s, n) 后再拼 3 字节的 "…"，实际长度可达
// n+3，usedBytes 按超额量累计——MaxBytes 上限被突破（预算成了软约束，
// 且自带测试先剥掉省略号再断言，坏实现全绿）。现在按 n-3 截再拼省略号，
// 返回值总长恒 ≤ n；n 装不下省略号（<3 字节）时只裁不标——预算是硬上限，
// 标记装不下就放弃标记。
func clip(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	const ellipsis = "…" // 3 字节
	// 与 hookengine 同一判据（stringx.TruncBytes）：按字节切会把多字节字符切成
	// 半个，产出的**非法 UTF-8** 会跟着注入体一起进模型上下文——而这里正是
	// 「不可信正文」的入口，坏字节在两侧都读不出原因。
	if n < len(ellipsis) {
		cut, _ := stringx.TruncBytes(s, n)
		return cut
	}
	cut, _ := stringx.TruncBytes(s, n-len(ellipsis))
	return cut + ellipsis
}
