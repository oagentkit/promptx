package filestate

// 2026-11-11 第二轮清零回归（#70）：读时正文未知 ≠ 内容一致。

import (
	"errors"
	"testing"
)

// TestUnknownContentNotTreatedAsIdentical_Issue70 同长同 mtime + 读时正文
// 未知（Content 留空）必须 fail-closed ErrStale——cp -p 回灌不再静默放行。
func TestUnknownContentNotTreatedAsIdentical_Issue70(t *testing.T) {
	tr := NewTracker()
	// 全量读但调用方未给正文（RecordRead 的 content 独立形参，可留空）。
	tr.RecordRead("/a.go", 0, 0, base, 100, "")
	// 同长同 mtime：正文未知 → 没法比对，不得放行。
	err := tr.CheckEdit("/a.go", base, 100, "poisoned-by-cp-p")
	if !errors.Is(err, ErrStale) {
		t.Fatalf("正文未知应 fail-closed stale（promptx#70），实得 %v", err)
	}
	// 真空文件（size 0、双侧空内容）：内容必然一致，放行不回退。
	tr.RecordRead("/empty.txt", 0, 0, base, 0, "")
	if err := tr.CheckEdit("/empty.txt", base, 0, ""); err != nil {
		t.Fatalf("零字节文件一致应放行: %v", err)
	}
	// 既有语义：正文已知且一致 → 豁免保持。
	tr.RecordRead("/b.go", 0, 0, base, 100, "known")
	if err := tr.CheckEdit("/b.go", base, 100, "known"); err != nil {
		t.Fatalf("已知一致应放行（回退检查）: %v", err)
	}
}
