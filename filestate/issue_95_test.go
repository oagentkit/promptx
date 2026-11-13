package filestate

// 2026-11-13 第三轮清零回归（#95）：#70 修复漏了「双空且 size>0」一格——
// `"" == ""` 也是相等，同长同 mtime 回灌、size 变化、外部清空三形态从
// 相等短路处静默放行（staleness 门整体失效 → 静默数据损坏）。

import (
	"errors"
	"testing"
	"time"
)

var base95 = base.Add(time.Hour) // 与其它用例的时间轴错开，防状态串扰

// TestDoubleEmptyWithSizeBiggerThanZeroFailsClosed_Issue95 实验1（探针形态）：
// 同长同 mtime + 双侧正文未保留（size=100 而 Content==""）——cp -p 回灌的
// 典型形态，此前 `""==""` 短路放行，编辑基于旧内容静默覆盖。
func TestDoubleEmptyWithSizeBiggerThanZeroFailsClosed_Issue95(t *testing.T) {
	tr := NewTracker()
	tr.RecordRead("/a.go", 0, 0, base95, 100, "")
	if err := tr.CheckEdit("/a.go", base95, 100, ""); !errors.Is(err, ErrStale) {
		t.Fatalf("双空且读时 size>0 应 fail-closed ErrStale（promptx#95），实得 %v", err)
	}
}

// TestSizeChangeWithUnknownContentFailsClosed_Issue95 实验2/3：包文档
// 「size 变化 → ErrStale」在正文未知时曾整体失效——size 100→120（前进）与
// 外部清空 100→0（前进）都从 `""==""` 的 formatter 豁免分支放行。
func TestSizeChangeWithUnknownContentFailsClosed_Issue95(t *testing.T) {
	later := base95.Add(5 * time.Millisecond)

	t.Run("size前进", func(t *testing.T) {
		tr := NewTracker()
		tr.RecordRead("/a.go", 0, 0, base95, 100, "")
		if err := tr.CheckEdit("/a.go", later, 120, ""); !errors.Is(err, ErrStale) {
			t.Fatalf("size 变化且正文未知应 ErrStale（promptx#95），实得 %v", err)
		}
	})

	t.Run("外部清空", func(t *testing.T) {
		tr := NewTracker()
		tr.RecordRead("/a.go", 0, 0, base95, 100, "")
		if err := tr.CheckEdit("/a.go", later, 0, ""); !errors.Is(err, ErrStale) {
			t.Fatalf("外部清空且读时正文未保留应 ErrStale（promptx#95），实得 %v", err)
		}
	})
}

// TestZeroByteAndKnownPathsUnchanged_Issue95 对照组：#95 收口不得回归
// #70 已认定的合法路径——零字节双空（size 0 佐证真·空）、已知一致豁免、
// formatter 重写豁免、已知不同与一侧空的 fail-closed 全部保持。
func TestZeroByteAndKnownPathsUnchanged_Issue95(t *testing.T) {
	later := base95.Add(5 * time.Millisecond)

	t.Run("零字节双空放行", func(t *testing.T) {
		tr := NewTracker()
		tr.RecordRead("/empty.txt", 0, 0, base95, 0, "")
		if err := tr.CheckEdit("/empty.txt", base95, 0, ""); err != nil {
			t.Fatalf("零字节文件双空应放行（size 0 佐证真·空）: %v", err)
		}
		// 零字节被 formatter 碰了一下（mtime 前进、size 仍 0）：内容仍真·一致。
		if err := tr.CheckEdit("/empty.txt", later, 0, ""); err != nil {
			t.Fatalf("零字节 mtime 前进但内容真·一致应豁免: %v", err)
		}
	})

	t.Run("已知一致放行", func(t *testing.T) {
		tr := NewTracker()
		tr.RecordRead("/b.go", 0, 0, base95, 100, "known")
		if err := tr.CheckEdit("/b.go", base95, 100, "known"); err != nil {
			t.Fatalf("已知一致应放行: %v", err)
		}
		// formatter 重写：mtime 前进、size 不变、已知内容一致 → 豁免保持。
		if err := tr.CheckEdit("/b.go", later, 100, "known"); err != nil {
			t.Fatalf("已知一致的 formatter 豁免应回归保持: %v", err)
		}
	})

	t.Run("已知不同拒绝", func(t *testing.T) {
		tr := NewTracker()
		tr.RecordRead("/c.go", 0, 0, base95, 100, "old")
		if err := tr.CheckEdit("/c.go", base95, 100, "new"); !errors.Is(err, ErrStale) {
			t.Fatalf("已知不同应 ErrStale，实得 %v", err)
		}
	})

	t.Run("一侧空拒绝", func(t *testing.T) {
		tr := NewTracker()
		tr.RecordRead("/d.go", 0, 0, base95, 100, "kept")
		if err := tr.CheckEdit("/d.go", base95, 100, ""); !errors.Is(err, ErrStale) {
			t.Fatalf("一侧空一侧非空应 ErrStale（#70 面保持），实得 %v", err)
		}
	})

	t.Run("已知内容外部清空拒绝", func(t *testing.T) {
		tr := NewTracker()
		tr.RecordRead("/e.go", 0, 0, base95, 100, "kept")
		if err := tr.CheckEdit("/e.go", later, 0, ""); !errors.Is(err, ErrStale) {
			t.Fatalf("已知内容被外部清空应 ErrStale，实得 %v", err)
		}
	})
}
