package memorypipe

// 裁剪判据（issue #201 修订）：
//   - rune 边界：注入体是不可信正文的入口，切坏字符产出的非法 UTF-8 会跟着
//     一起进模型上下文，而两侧都读不出原因；
//   - **预算含省略号**：MaxBytes 是硬上限，裁剪结果（含省略号）不得突破
//     ——旧实现 TruncBytes(n) 再拼 3 字节省略号，实际可达 n+3，而旧测试
//     先剥掉省略号再断言，把预算契约在测试里绕过了（坏实现全绿）；
//   - Reason 语义：字节预算耗尽必须报 MaxBytes，不得落进 MaxItems 兜底
//     （宿主照着错误提示调 MaxItems 永远修不好）。

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestClipKeepsUTF8Boundary rune 边界 + 预算含省略号。
func TestClipKeepsUTF8Boundary(t *testing.T) {
	// 每个汉字 3 字节 ⇒ 预算 100 = 3*33 + 1，落在第 34 个字符的中间。
	got := clip(strings.Repeat("忆", 100), 100)
	if !utf8.ValidString(got) {
		t.Fatalf("裁剪产出了非法 UTF-8（%d 字节）: %q", len(got), got)
	}
	// issue #201 的核心断言：**含省略号的整串**不得突破预算——不再先剥掉
	// 省略号再断言（那是把契约在测试里绕过的旧写法）。
	if len(got) > 100 {
		t.Fatalf("裁剪结果 %d 字节（含省略号），超出预算 100", len(got))
	}
	body := strings.TrimSuffix(got, "…")
	// 预算 100 减去 3 字节省略号后按 rune 回退：留 32 个完整字符（96 字节）
	// + 省略号 = 99 ≤ 100。（旧实现 TruncBytes(100) 后硬拼省略号 = 102，
	// 突破预算——正是 #201 的缺陷形态。）
	if n := utf8.RuneCountInString(body); n != 32 {
		t.Fatalf("边界回退应留下 32 个完整字符，实得 %d", n)
	}

	// 对照：没超预算原样返回，不追加省略号（否则每条记忆都带尾巴）。
	if out := clip("abc", 3); out != "abc" {
		t.Fatalf("未超预算不该改动，实得 %q", out)
	}
}

// TestClipBudgetIsHardCap 各种预算边界下返回值总长（含省略号）恒 ≤ n。
func TestClipBudgetIsHardCap(t *testing.T) {
	body := strings.Repeat("忆知", 50) // 200 字节的中文
	for _, n := range []int{1, 2, 3, 4, 5, 6, 7, 97, 100, 199, 200, 201} {
		got := clip(body, n)
		if len(got) > n {
			t.Fatalf("n=%d: 裁剪结果 %d 字节突破预算", n, len(got))
		}
		if !utf8.ValidString(got) {
			t.Fatalf("n=%d: 裁剪产出非法 UTF-8", n)
		}
	}
	// n<3（省略号装不下）：只裁不标，预算仍不破。
	if got := clip("abcdef", 2); len(got) > 2 {
		t.Fatalf("n=2: 实得 %d 字节", len(got))
	}
	// 装得下省略号时带标记（裁剪显式——模型/宿主看得出这不是全文）。
	if got := clip(strings.Repeat("a", 10), 8); !strings.HasSuffix(got, "…") || len(got) > 8 {
		t.Fatalf("n=8 裁剪应带省略号标记且不超预算，实得 %q（%d 字节）", got, len(got))
	}
}

// 端到端：MaxBytes 落在多字节字符中间时，注入片段与渲染结果都必须是合法 UTF-8。
func TestBuildMaxBytesKeepsUTF8Boundary(t *testing.T) {
	res, err := Build([]Entry{
		entry("m1", strings.Repeat("忆", 40), "scope", "s1", base), // 120 字节
	}, Query{Scope: "scope", MaxBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated {
		t.Fatal("超出 MaxBytes 必须标 truncated")
	}
	if len(res.Pieces) != 1 {
		t.Fatalf("应注入 1 条，实得 %d", len(res.Pieces))
	}
	if !utf8.ValidString(res.Pieces[0].Text) {
		t.Fatal("注入片段含非法 UTF-8")
	}
	if !utf8.ValidString(res.Prompt()) {
		t.Fatal("渲染结果含非法 UTF-8")
	}
}

// TestBuildMaxBytesBudgetNeverExceeded 验收一（issue #201）：MaxBytes=100 +
// 长中文正文 ⇒ 注入正文总字节 ≤ 100（issue 原文触发面：旧实现注入 102 字节）。
func TestBuildMaxBytesBudgetNeverExceeded(t *testing.T) {
	res, err := Build([]Entry{
		entry("m1", strings.Repeat("忆", 40), "scope", "s1", base), // 120 字节
		entry("m2", strings.Repeat("知", 30), "scope", "s2", base), // 90 字节
	}, Query{Scope: "scope", MaxBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pieces) == 0 {
		t.Fatal("预算装得下至少一条，不得全灭")
	}
	total := 0
	for _, p := range res.Pieces {
		total += len(p.Text)
		if !utf8.ValidString(p.Text) {
			t.Fatalf("注入片段含非法 UTF-8: %q", p.Text)
		}
	}
	if total > 100 {
		t.Fatalf("注入正文总字节 %d 突破 MaxBytes=100", total)
	}
	if !res.Truncated || !strings.Contains(res.Reason, "MaxBytes") {
		t.Fatalf("截断原因应指向 MaxBytes: truncated=%v reason=%q", res.Truncated, res.Reason)
	}
}

// TestBuildReasonReportsMaxBytesWhenBudgetExhausted 验收二（issue #201）：
// 字节预算耗尽（remaining 装不下更多条目）时 Reason 必须含 MaxBytes——
// 旧实现不设 Reason，落进「候选数超过 MaxItems」兜底，排障方向直接错。
func TestBuildReasonReportsMaxBytesWhenBudgetExhausted(t *testing.T) {
	res, err := Build([]Entry{
		entry("m1", strings.Repeat("a", 100), "scope", "s1", base), // 恰好占满 100
		entry("m2", "第二条", "scope", "s2", base),                    // 预算已尽 ⇒ 丢弃
	}, Query{Scope: "scope", MaxBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pieces) != 1 {
		t.Fatalf("预算恰装一条，实得 %d 条", len(res.Pieces))
	}
	if !res.Truncated {
		t.Fatal("预算耗尽的丢弃必须标 truncated")
	}
	if !strings.Contains(res.Reason, "MaxBytes") {
		t.Fatalf("字节预算耗尽必须报 MaxBytes（旧实现在此报成 MaxItems），实得 %q", res.Reason)
	}
	if strings.Contains(res.Reason, "MaxItems") {
		t.Fatalf("字节预算耗尽不得报成 MaxItems，实得 %q", res.Reason)
	}
	for _, d := range res.Dropped {
		if d.ID == "m2" && d.Why != "超过 MaxBytes" {
			t.Fatalf("被丢条目的原因应指向 MaxBytes，实得 %q", d.Why)
		}
	}
}
