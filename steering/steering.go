// Package steering 运行中转向队列：turn 执行中用户插入的新输入按投递语义
// 分流——guide（下一个模型请求边界前注入，不打断当前工具执行）与 queue
// （本 turn 结束后作为下一轮输入）。对标 ZCode steering.ts 的 pendingInputs
// 形态；不含 turn 机集成（那是运行时的事），只提供线程安全的队列原语。
//
// 关键语义（ZCode 踩坑固化的）：
//   - guide 输入不丢：模型执行窗口已关闭时（Drain 后未消费）fallback 为
//     queue 语义——**库内可执行**（promptx#71）：DrainGuides 摘走的条目登记
//     在 in-flight 未确认集合里，CommitGuides 确认消费；DemoteGuidesToQueued
//     把未确认的按**原位次**归队为 queued（手工 Enqueue 会打乱 FIFO 位次，
//     先 Steer 后 Enqueue 的顺序在降级后必须保持）；
//   - 同一 turn 内 guide 注入后续跑而非新开 turn，保持上下文连续。
package steering

import "sync"

// Delivery 投递语义。
type Delivery string

const (
	// Guide 下一模型请求边界注入（不中断当前工具执行）。
	Guide Delivery = "guide"
	// Queued 本 turn 结束后作为下一轮输入。
	Queued Delivery = "queue"
)

// Input 一条待投递的用户输入。
type Input struct {
	Text     string
	Delivery Delivery
}

// pendingEntry 队列内部条目：seq 是 push 序（跨 Delivery 的 FIFO 位次单源，
// guide 降级归队按它插回原位——promptx#71）。
type pendingEntry struct {
	seq int
	in  Input
}

// Queue 待投递输入队列（并发安全）。零值可用。
type Queue struct {
	mu      sync.Mutex
	seq     int
	pending []pendingEntry
	// drained 已被 DrainGuides 取走但**未确认消费**的 guide（in-flight）：
	// CommitGuides 确认后清空；DemoteGuidesToQueued 把它们按原位次归队
	// （promptx#71——「Drain 后未消费 → fallback 为 queue 语义」的库内实现）。
	drained []pendingEntry
}

// Steer 投递 guide 输入（模型请求边界注入）。
func (q *Queue) Steer(text string) {
	q.push(Input{Text: text, Delivery: Guide})
}

// Enqueue 投递 queued 输入（turn 结束后消费）。
func (q *Queue) Enqueue(text string) {
	q.push(Input{Text: text, Delivery: Queued})
}

func (q *Queue) push(in Input) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seq++
	q.pending = append(q.pending, pendingEntry{seq: q.seq, in: Input{Text: in.Text, Delivery: in.Delivery}})
}

// DrainGuides 取走全部 guide 输入（模型请求边界调用）。模型执行窗口关闭时
// guide 降级为 queued——不丢输入。
func (q *Queue) DrainGuides() []Input {
	q.mu.Lock()
	defer q.mu.Unlock()
	var guides []Input
	var rest []pendingEntry
	for _, pe := range q.pending {
		if pe.in.Delivery == Guide {
			guides = append(guides, pe.in)
			q.drained = append(q.drained, pe)
		} else {
			rest = append(rest, pe)
		}
	}
	q.pending = rest
	return guides
}

// CommitGuides 确认最近一次 DrainGuides 取走的 guide 已全部消费（清空
// in-flight 集合——promptx#71 的消费确认半边）。
func (q *Queue) CommitGuides() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.drained = nil
}

// DequeueQueued 取一条 queued 输入（FIFO）。队列空返回 ok=false。
func (q *Queue) DequeueQueued() (Input, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, pe := range q.pending {
		if pe.in.Delivery == Queued {
			q.pending = append(q.pending[:i:i], q.pending[i+1:]...)
			return pe.in, true
		}
	}
	return Input{}, false
}

// DemoteGuidesToQueued 把全部 guide 降级为 queued（turn 提前收尾/执行窗口
// 关闭时调用——guide 不丢，转排队）。
func (q *Queue) DemoteGuidesToQueued() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.pending {
		if q.pending[i].in.Delivery == Guide {
			q.pending[i].in.Delivery = Queued
		}
	}
	// in-flight 未确认的 guide 同样归队（promptx#71）：按 push 序插回原位
	// ——手工 Enqueue 会排到队尾打乱 FIFO（先 Steer 后 Enqueue 的顺序在
	// 降级后必须保持，A 组位次实验钉的语义）。
	if len(q.drained) > 0 {
		for _, de := range q.drained {
			de.in.Delivery = Queued
			idx := len(q.pending)
			for i, pe := range q.pending {
				if pe.seq > de.seq {
					idx = i
					break
				}
			}
			q.pending = append(q.pending, pendingEntry{})
			copy(q.pending[idx+1:], q.pending[idx:])
			q.pending[idx] = de
		}
		q.drained = nil
	}
}

// Len 当前待投递总数（观测用）。
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func filterPending(pending []pendingEntry, keep func(pendingEntry) bool) []pendingEntry {
	out := pending[:0:0]
	for _, pe := range pending {
		if keep(pe) {
			out = append(out, pe)
		}
	}
	return out
}
