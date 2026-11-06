package sysprompt

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestBuildOrderingAndBlocks(t *testing.T) {
	b := &Builder{}
	b.Add(NewSection("dyn-a", "", "", "动态A"))                         // system/dynamic（缺省）
	b.Add(IdentitySection("你是审查助手"))                                  // system/stable
	b.Add(NewSection("dyn-b", "", "", "动态B"))                         // system/dynamic
	b.Add(DateSection(time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC))) // user_context/dynamic
	res := b.Build()

	if len(res.System) != 2 {
		t.Fatalf("应 2 块: %d", len(res.System))
	}
	if res.System[0].Boundary != Stable || !strings.Contains(res.System[0].Content, "审查助手") {
		t.Fatalf("首块应为 stable 身份: %+v", res.System[0])
	}
	if res.System[1].Boundary != Dynamic {
		t.Fatalf("次块应为 dynamic: %+v", res.System[1])
	}
	if !strings.Contains(res.System[1].Content, "动态A") || !strings.Contains(res.System[1].Content, "动态B") {
		t.Fatal("dynamic 段应按插入序拼入")
	}
	// 排序面：stable 段在 Sections 首位
	if res.Sections[0].Name != "Identity" {
		t.Fatalf("stable 应排首位: %v", res.Sections[0].Name)
	}
	// user_context 含日期 + 免责
	if !strings.Contains(res.UserContext, "Today's date is 2026-09-27") ||
		!strings.Contains(res.UserContext, DefaultUserContextDisclaimer) {
		t.Fatalf("user_context 拼装错误: %q", res.UserContext)
	}
	// 计量 = 各段之和
	var chars int
	for _, s := range res.Sections {
		chars += s.Chars
	}
	if res.TotalChars != chars || res.TotalTokens == 0 {
		t.Fatalf("计量: chars=%d total=%d", chars, res.TotalChars)
	}
}

func TestBuildDeterministicAndFrozen(t *testing.T) {
	b := &Builder{}
	b.Add(IdentitySection("身份"))
	b.Add(NewSection("env", "", "", "env 内容"))
	r1 := b.Build()
	b.Add(NewSection("late", "", "", "后加段"))
	r2 := b.Build()
	if strings.Contains(r1.UserContext+r1.System[1].Content, "后加段") {
		t.Fatal("Build 产物应冻结")
	}
	if !strings.Contains(r2.System[1].Content, "后加段") {
		t.Fatal("新 Build 应含后加段")
	}
}

func TestBuildEmpty(t *testing.T) {
	res := (&Builder{}).Build()
	if len(res.System) != 0 || res.UserContext != "" || res.TotalChars != 0 {
		t.Fatalf("空构建: %+v", res)
	}
}

func TestSectionMetering(t *testing.T) {
	// 计量走 CJK 感知口径（promptx#30）：纯 ASCII「abc」= 3 单位 → 3/3+1 = 2
	//（该口径非空下限 1 再 +1，短文本上略偏大，长文本可忽略——与 llm 预算记账
	// 同源比「短文本更准但中文系统性低估一半」重要得多）。
	s := NewSection("x", "", "", "abc")
	if s.Chars != 3 || s.Tokens != 2 {
		t.Fatalf("计量: %d/%d, want 3/2", s.Chars, s.Tokens)
	}
	s2 := NewSection("y", "", "", "abcd") // 4 单位 → 4/3+1 = 2
	if s2.Tokens != 2 {
		t.Fatalf("向上取整: %d", s2.Tokens)
	}
	// 缺省 target/boundary
	if s.Target != TargetSystem || s.Boundary != Dynamic {
		t.Fatal("缺省应 system/dynamic")
	}
}

func TestEnvSection(t *testing.T) {
	s := EnvSection(EnvInfo{
		Cwd: "/repo", IsGitRepo: true, Platform: "darwin",
		Shell: "/bin/zsh", OSVersion: "Darwin 27.0.0", ModelID: "bigmodel/glm-5.3",
	})
	for _, want := range []string{"Primary working directory: /repo", "Is a git repository: yes",
		"Platform: darwin", "Shell: /bin/zsh", "OS Version: Darwin 27.0.0", "bigmodel/glm-5.3"} {
		if !strings.Contains(s.Content, want) {
			t.Errorf("env 段缺 %q:\n%s", want, s.Content)
		}
	}
	if s.Boundary != Dynamic || s.Target != TargetSystem {
		t.Fatal("env 段应 system/dynamic")
	}
}

func TestGitSection(t *testing.T) {
	s := GitSection(GitInfo{
		Branch: "main", MainBranch: "main", User: "MurphyYi",
		StatusLines:   []string{" M a.go", "?? b.go"},
		RecentCommits: []string{"abc123 fix", "def456 feat"},
	})
	for _, want := range []string{"snapshot in time", "Current branch: main",
		"Main branch (you will usually use this for PRs): main", "Git user: MurphyYi",
		" M a.go", "abc123 fix"} {
		if !strings.Contains(s.Content, want) {
			t.Errorf("git 段缺 %q:\n%s", want, s.Content)
		}
	}
	// clean 形态
	s2 := GitSection(GitInfo{Branch: "main"})
	if !strings.Contains(s2.Content, "(clean)") {
		t.Fatalf("无状态行应 clean:\n%s", s2.Content)
	}
	// unknown 形态
	s3 := GitSection(GitInfo{Branch: "main", StatusUnknown: true})
	if !strings.Contains(s3.Content, "(unknown)") {
		t.Fatalf("采集失败应 unknown:\n%s", s3.Content)
	}
}

func TestDetectEnvInGitRepo(t *testing.T) {
	// agentkit 仓库自身就是 git 仓——采集应得出分支与提交
	env, git, err := DetectEnv(context.Background(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if !env.IsGitRepo || git == nil {
		t.Fatalf("应识别 git 仓: %+v %+v", env, git)
	}
	if env.Platform == "" || env.Shell == "" {
		t.Fatalf("平台/shell 应有值: %+v", env)
	}
	if git.Branch == "" || len(git.RecentCommits) == 0 {
		t.Fatalf("分支与提交应采集到: %+v", git)
	}
}

func TestDetectEnvNotARepo(t *testing.T) {
	env, git, err := DetectEnv(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if env.IsGitRepo || git != nil {
		t.Fatalf("临时目录非 git 仓: %+v %+v", env, git)
	}
}

func TestUserContextCustomWording(t *testing.T) {
	b := &Builder{UserContextIntro: "参考材料：", UserContextDisclaimer: "免责"}
	b.Add(DateSection(time.Now()))
	res := b.Build()
	if !strings.HasPrefix(res.UserContext, "参考材料：") || !strings.HasSuffix(res.UserContext, "免责") {
		t.Fatalf("覆写措辞未生效: %q", res.UserContext)
	}
}

// TestSectionTokensUseCJKOracle 中文段按 CJK 感知口径计量（promptx#30）。
//
// 判别式在两个口径的**比值**上，不是绝对值上：同一段中文下，
// 纯 rune 口径 = ceil(runes/3)，CJK 口径 = ceil(2*runes/3)+1 ⇒ 约 2 倍。
// 断言「Tokens 必须 ≥ runes*2/3」——退回 EstTokens 时 12 个汉字只得 4，
// 而这里要求 ≥8，直接红。绝对值钉不死口径（两种口径在短 ASCII 上几乎重合）。
func TestSectionTokensUseCJKOracle(t *testing.T) {
	const cn = "提示词中文内容需要按汉字口径估算" // 16 个汉字
	runes := utf8.RuneCountInString(cn)
	s := NewSection("cn", "", "", cn)
	if s.Chars != runes {
		t.Fatalf("Chars = %d, want %d", s.Chars, runes)
	}
	// CJK 口径：每个汉字 2 单位 → units = 2*runes，tokens = units/3 + 1
	want := 2*runes/3 + 1
	if s.Tokens != want {
		t.Fatalf("中文段 Tokens = %d, want %d（CJK 口径）——纯 rune 口径只会给 %d，"+
			"与 llm 预算记账差约 2 倍", s.Tokens, want, (runes+2)/3)
	}
	if s.Tokens <= (runes+2)/3 {
		t.Fatalf("Tokens = %d 不大于纯 rune 口径 %d——口径没切过来", s.Tokens, (runes+2)/3)
	}
}
