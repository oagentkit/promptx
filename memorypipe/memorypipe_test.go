package memorypipe

import (
	"strings"
	"testing"
	"time"
)

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func entry(id, text, scope, source string, at time.Time, tags ...string) Entry {
	return Entry{ID: id, Text: text, Scope: scope, Source: source, At: at, Tags: tags}
}

// TestMissingScopeIsRejected 作用域不明的注入直接报错：
// 「该不该生效」无从判定时不许猜。
func TestMissingScopeIsRejected(t *testing.T) {
	if _, err := Build(nil, Query{}); err == nil {
		t.Fatal("没有 scope 的查询必须报错")
	}
}

// TestSourceIsMandatory 没有来源的条目**不注入**，而进 Dropped 并说明。
//
// 没有来源的记忆与模型的猜测在文本上无法分辨——这是本包存在的一半理由。
func TestSourceIsMandatory(t *testing.T) {
	res, err := Build([]Entry{entry("m1", "无来源的结论", "project", "", base)}, Query{Scope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pieces) != 0 {
		t.Fatal("没有来源的条目不得注入")
	}
	if len(res.Dropped) != 1 || !strings.Contains(res.Dropped[0].Why, "没有来源") {
		t.Fatalf("必须显式丢弃并说明原因，实得 %+v", res.Dropped)
	}
}

// TestScopeDoesNotFallBack 作用域精确匹配，不做父级兜底。
//
// 兜底会让「作用域」失去意义：只该在某个项目里生效的记忆会在任何会话里生效。
func TestScopeDoesNotFallBack(t *testing.T) {
	entries := []Entry{entry("g1", "全局结论", "global", "s", base)}
	res, _ := Build(entries, Query{Scope: "project"})
	if len(res.Pieces) != 0 || !strings.Contains(res.Dropped[0].Why, "作用域不匹配") {
		t.Fatalf("scope 不匹配必须丢弃并说明，实得 %+v", res)
	}
}

// TestTagsAreAnd 标签按 AND：OR 会让「加个标签」变成放宽而不是收紧。
func TestTagsAreAnd(t *testing.T) {
	entries := []Entry{entry("m1", "正文", "project", "s", base, "a")}
	res, _ := Build(entries, Query{Scope: "project", Tags: []string{"a", "b"}})
	if len(res.Pieces) != 0 || !strings.Contains(res.Dropped[0].Why, "缺少标签 b") {
		t.Fatalf("缺一个标签就必须丢弃，实得 %+v", res)
	}
	res2, _ := Build(entries, Query{Scope: "project", Tags: []string{"a"}})
	if len(res2.Pieces) != 1 {
		t.Fatalf("全部标签命中必须注入，实得 %+v", res2)
	}
}

// TestLatestWinsForSameID 同 ID 只留最新：两条正文不同的同 ID 记忆会让
// 读者只能猜哪条是当前的。
func TestLatestWinsForSameID(t *testing.T) {
	entries := []Entry{
		entry("m1", "旧结论", "project", "s", base),
		entry("m1", "新结论", "project", "s", base.Add(time.Hour)),
	}
	res, _ := Build(entries, Query{Scope: "project"})
	if len(res.Pieces) != 1 || res.Pieces[0].Text != "新结论" {
		t.Fatalf("同 ID 必须留最新，实得 %+v", res.Pieces)
	}
}

// TestTruncationIsExplicit 预算裁剪必须显式。
func TestTruncationIsExplicit(t *testing.T) {
	entries := []Entry{
		entry("m1", "aaaa", "project", "s", base.Add(2*time.Hour)),
		entry("m2", "bbbb", "project", "s", base.Add(time.Hour)),
	}
	res, _ := Build(entries, Query{Scope: "project", MaxItems: 1})
	if len(res.Pieces) != 1 || !res.Truncated || len(res.Dropped) != 1 {
		t.Fatalf("MaxItems 裁剪必须显式，实得 %+v", res)
	}
	res2, _ := Build(entries, Query{Scope: "project"})
	if res2.Truncated {
		t.Fatal("没超预算不该标截断")
	}
	// 字节预算：只够第一条（4 字节），第二条整条放不下 ⇒ 丢弃并标截断。
	res3, _ := Build(entries, Query{Scope: "project", MaxBytes: 4})
	if len(res3.Pieces) != 1 || !res3.Truncated {
		t.Fatalf("MaxBytes 必须生效，实得 %+v", res3)
	}
	if !strings.Contains(res3.Dropped[0].Why, "MaxBytes") {
		t.Fatalf("丢弃原因必须指向预算，实得 %+v", res3.Dropped)
	}
}

// TestPromptCarriesMarks 注入体必须逐条打标：id / source / at 三样缺一不可。
func TestPromptCarriesMarks(t *testing.T) {
	res, _ := Build([]Entry{entry("m1", "结论", "project", "session-42", base)}, Query{Scope: "project"})
	out := res.Prompt()
	for _, want := range []string{"<memory>", "id=m1", "source=session-42", "at=2026-01-01T00:00:00Z", "结论"} {
		if !strings.Contains(out, want) {
			t.Fatalf("注入体缺少 %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "已按预算裁剪") {
		t.Fatal("没裁剪不该出现裁剪标记")
	}
	// 空结果渲染成空串（不要一个空标签）。
	if got := (Result{}).Prompt(); got != "" {
		t.Fatalf("空结果不该渲染内容，实得 %q", got)
	}
	// 裁剪过就必须在注入体里说出来（模型据此知道「还有别的」）。
	trunc, _ := Build([]Entry{
		entry("m1", "aaa", "project", "s", base.Add(time.Hour)),
		entry("m2", "bbb", "project", "s", base),
	}, Query{Scope: "project", MaxItems: 1})
	if !strings.Contains(trunc.Prompt(), "已按预算裁剪") {
		t.Fatal("裁剪后注入体必须标出来")
	}
}

// TestBuildDeterministicForEmptyIDEntries 空 ID 同时刻条目的输出必须逐字可复现
// （issue #203）：此前比较器回退读条目 ID，两条空 ID 条目恒比出 0，
// slices.SortFunc 非稳定——输出顺序随 map 迭代序漂移，配 MaxItems 时
// 哪条被注入随运行变化。回退改用去重键后，序恒为键升序（"text:A" < "text:B"）。
func TestBuildDeterministicForEmptyIDEntries(t *testing.T) {
	q := Query{Scope: "project", MaxItems: 1}
	entries := []Entry{
		entry("", "B", "project", "src", base),
		entry("", "A", "project", "src", base),
	}
	var first string
	var last Result
	for i := 0; i < 50; i++ {
		res, err := Build(entries, q)
		if err != nil {
			t.Fatal(err)
		}
		out := res.Prompt()
		if first == "" {
			first = out
		} else if out != first {
			t.Fatalf("输出随运行漂移（注入顺序随 map 迭代序）:\nfirst=%q\ngot=%q", first, out)
		}
		last = res
	}
	if len(last.Pieces) != 1 || last.Pieces[0].Entry.Text != "A" {
		t.Fatalf("去重键升序应注入 A: %+v", last.Pieces)
	}
	if len(last.Dropped) != 1 || last.Dropped[0].Text == "" && last.Dropped[0].Why == "" {
		t.Fatalf("另一条应被 MaxItems 裁掉并说明: %+v", last.Dropped)
	}
}

// TestPromptMarksEmptyID 空 ID 条目的渲染必须显式标注「无 id」（issue #203）：
// 渲染一个空的 id= 槽位会让模型拿空串当引用锚点，且分不清「这条没有 id」
// 与「标记坏了」。
func TestPromptMarksEmptyID(t *testing.T) {
	res, err := Build([]Entry{entry("", "正文", "project", "src", base)}, Query{Scope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if out := res.Prompt(); !strings.Contains(out, "id=<无id>") {
		t.Fatalf("空 ID 应显式标注，实得: %q", out)
	}
}

// TestBuildClonesEntryTags Build 产出的 Piece.Entry 不得与调用方入参共享
// Tags 底层数组（issue #394）。
//
// Entry 自称「值类型」，但 Tags 是切片：按值放进 Piece 只复制切片头。
// 标签是 AND 匹配的判据，改穿后两边的匹配结果随之改变。
func TestBuildClonesEntryTags(t *testing.T) {
	entries := []Entry{entry("m1", "正文", "project", "s", base, "alpha")}
	res, err := Build(entries, Query{Scope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pieces) != 1 {
		t.Fatalf("应注入 1 条，实得 %+v", res)
	}
	res.Pieces[0].Entry.Tags[0] = "mutated-out"
	if entries[0].Tags[0] != "alpha" {
		t.Fatalf("Build 的结果与入参共享 Tags 底层数组（改结果写穿源）: %v", entries[0].Tags)
	}

	entries2 := []Entry{entry("m2", "正文", "project", "s", base, "alpha")}
	res2, err := Build(entries2, Query{Scope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	entries2[0].Tags[0] = "mutated-in"
	if res2.Pieces[0].Entry.Tags[0] != "alpha" {
		t.Fatalf("Build 的结果与入参共享 Tags 底层数组（改源写穿结果）: %v", res2.Pieces[0].Entry.Tags)
	}
}
