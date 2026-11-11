package steering

// 2026-11-11 第二轮清零回归（#71）：Drain 后未消费的 guide 按原位次归队。

import "testing"

// TestDrainedGuidesDemotedWithFIFOOrder_Issue71 Drain 后窗口关闭 → 未确认
// guide 按原位次降级归队（手工 Enqueue 的队尾插入会打乱 FIFO——先 Steer 后
// Enqueue 的顺序在降级后必须保持）。
func TestDrainedGuidesDemotedWithFIFOOrder_Issue71(t *testing.T) {
	q := &Queue{}
	q.Steer("guide-1")
	q.Enqueue("queued-1")
	q.Steer("guide-2")

	guides := q.DrainGuides()
	if len(guides) != 2 {
		t.Fatalf("应取走 2 条 guide: %+v", guides)
	}
	if q.Len() != 1 {
		t.Fatalf("队列应剩 1 条 queued: %d", q.Len())
	}
	// 窗口关闭：未确认的 drain 产物按原位次归队为 queued。
	q.DemoteGuidesToQueued()
	var texts []string
	for {
		in, ok := q.DequeueQueued()
		if !ok {
			break
		}
		texts = append(texts, in.Text)
	}
	want := []string{"guide-1", "queued-1", "guide-2"}
	if len(texts) != len(want) {
		t.Fatalf("归队后应有 3 条: %v", texts)
	}
	for i := range want {
		if texts[i] != want[i] {
			t.Fatalf("FIFO 位次错乱: got %v want %v", texts, want)
		}
	}
}

// TestCommitGuidesClearsInflight_Issue71 消费确认后 in-flight 清空——降级
// 不再重复归队。
func TestCommitGuidesClearsInflight_Issue71(t *testing.T) {
	q := &Queue{}
	q.Steer("g1")
	q.Steer("g2")
	_ = q.DrainGuides()
	q.CommitGuides()
	q.DemoteGuidesToQueued()
	if q.Len() != 0 {
		t.Fatalf("已确认消费的 guide 不应归队: %d", q.Len())
	}
}

// TestDrainAfterDemoteNoDoubleReturn_Issue71 归队与确认混用：部分确认场景
// 收口不重复。
func TestDrainAfterDemoteNoDoubleReturn_Issue71(t *testing.T) {
	q := &Queue{}
	q.Steer("g1")
	_ = q.DrainGuides()
	q.DemoteGuidesToQueued() // g1 归队
	in, ok := q.DequeueQueued()
	if !ok || in.Text != "g1" || in.Delivery != Queued {
		t.Fatalf("归队应为 queued: %+v ok=%v", in, ok)
	}
	// 再 Drain：无 guide 可取，无残留。
	if got := q.DrainGuides(); len(got) != 0 {
		t.Fatalf("不应重复取: %+v", got)
	}
}
