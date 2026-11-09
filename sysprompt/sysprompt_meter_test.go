package sysprompt

import (
	"testing"
	"unicode/utf8"

	"github.com/oagentkit/textx/textutil"
)

// TestBuildBareSectionMeterBackfilled 回归（#57）：裸构造 Section{Name, Content}
// （Chars/Tokens 零值）进 Sections/进块组装生效，但 Build 的累加循环只加自报
// 计量 ⇒ TotalChars/TotalTokens 贡献恒 0，预算闸门按 TotalTokens 少算整段。
// 修后 Build 兜底补算：口径与 NewSection 完全一致（utf8.RuneCountInString +
// EstTokensCJK），且补算结果回写进 Sections（观测面同一份事实）。
func TestBuildBareSectionMeterBackfilled(t *testing.T) {
	const content = "裸构造的系统提示词段落，预算闸门必须看得见这段。"
	b := &Builder{}
	b.Add(Section{Name: "bare", Content: content})
	res := b.Build()

	want := NewSection("ref", TargetSystem, Dynamic, content)
	if res.TotalChars != want.Chars || res.TotalTokens != want.Tokens {
		t.Fatalf("裸段计量未按 NewSection 同口径计入 Total: got %d/%d, want %d/%d",
			res.TotalChars, res.TotalTokens, want.Chars, want.Tokens)
	}
	if len(res.Sections) != 1 {
		t.Fatalf("Sections: %d", len(res.Sections))
	}
	got := res.Sections[0]
	if got.Chars != want.Chars || got.Tokens != want.Tokens {
		t.Fatalf("补算未回写 Sections: got %d/%d, want %d/%d", got.Chars, got.Tokens, want.Chars, want.Tokens)
	}
	if utf8.RuneCountInString(content) != want.Chars || want.Tokens != textutil.EstTokensCJK(content) {
		t.Fatalf("基准口径漂移: %d %d", want.Chars, want.Tokens)
	}
}

// TestBuildMixedMeter 补算与自报计量共存：NewSection 段（自报）+ 裸段（兜底）
// 混合组装，Total = 各段之和，既有段计量不受兜底逻辑改写。
func TestBuildMixedMeter(t *testing.T) {
	ref1 := NewSection("a", TargetSystem, Stable, "身份纪律内容甲")
	ref2 := NewSection("b", TargetSystem, Dynamic, "环境快照内容乙")
	b := &Builder{}
	b.Add(ref1)
	b.Add(Section{Name: "bare", Content: "裸段丙"})
	b.Add(ref2)
	res := b.Build()

	wantChars := ref1.Chars + ref2.Chars + utf8.RuneCountInString("裸段丙")
	wantTokens := ref1.Tokens + ref2.Tokens + textutil.EstTokensCJK("裸段丙")
	if res.TotalChars != wantChars || res.TotalTokens != wantTokens {
		t.Fatalf("混合计量: got %d/%d, want %d/%d", res.TotalChars, res.TotalTokens, wantChars, wantTokens)
	}
}

// TestBuildSelfReportedMeterRespected 反证：自报计量（调用方显式设 Chars/Tokens，
// 口径由调用方负责）Build 不得改写——兜底只补零值，不覆盖既有值。
func TestBuildSelfReportedMeterRespected(t *testing.T) {
	b := &Builder{}
	b.Add(Section{Name: "self", Content: "自定义口径内容", Chars: 42, Tokens: 7})
	res := b.Build()
	if res.TotalChars != 42 || res.TotalTokens != 7 {
		t.Fatalf("自报计量被改写: %d/%d", res.TotalChars, res.TotalTokens)
	}
	if res.Sections[0].Chars != 42 || res.Sections[0].Tokens != 7 {
		t.Fatalf("Sections 自报计量被改写: %+v", res.Sections[0])
	}
}

// TestBuildHalfMeterBackfilled 反证：只填一半计量（Chars 有值、Tokens 漏 0）
// 同样按零值兜底——漏的那半补算，已有的那半不动。
func TestBuildHalfMeterBackfilled(t *testing.T) {
	const content = "只报了字符数的段落"
	b := &Builder{}
	b.Add(Section{Name: "half", Content: content, Chars: 9})
	res := b.Build()
	if res.TotalChars != 9 {
		t.Fatalf("已有 Chars 被改写: %d", res.TotalChars)
	}
	if res.TotalTokens != textutil.EstTokensCJK(content) {
		t.Fatalf("漏报 Tokens 未兜底: %d, want %d", res.TotalTokens, textutil.EstTokensCJK(content))
	}
}

// TestBuildEmptyContentNoMeter 反证：空内容段零计量是合法状态，兜底不得凭空
// 造数（Content == "" ⇒ 不补）。
func TestBuildEmptyContentNoMeter(t *testing.T) {
	b := &Builder{}
	b.Add(Section{Name: "empty"})
	b.Add(NewSection("real", TargetSystem, Dynamic, "实际内容"))
	res := b.Build()
	if res.TotalChars != utf8.RuneCountInString("实际内容") || res.TotalTokens == 0 {
		t.Fatalf("空段计量被污染: %d/%d", res.TotalChars, res.TotalTokens)
	}
}
