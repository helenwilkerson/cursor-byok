package forwarder

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

const appendSequenceRetention = 10 * time.Minute

type appendSequenceTracker struct {
	mu     sync.Mutex
	states map[string]*appendSequenceState
}

type appendSequenceState struct {
	mu         sync.Mutex
	next       int64
	processing bool
	ready      chan struct{}
	updatedAt  time.Time
}

type appendSequenceTicket struct {
	state *appendSequenceState
	seq   int64
}

func newAppendSequenceTracker() *appendSequenceTracker {
	return &appendSequenceTracker{
		states: make(map[string]*appendSequenceState),
	}
}

// Acquire 为指定请求序列申请独占处理票据，并识别过期或等待取消的消息。
// 参数 ctx 用于取消等待，requestID 标识请求，appendSeq 表示消息序号；返回票据、是否过期及错误。
// lyh用cursor修改 2026-08-01：向序列状态传递 request_id，支持记录和识别 Cursor 复用请求的新轮次
func (tracker *appendSequenceTracker) Acquire(ctx context.Context, requestID string, appendSeq int64) (appendSequenceTicket, bool, error) {
	if tracker == nil || strings.TrimSpace(requestID) == "" || appendSeq <= 0 {
		return appendSequenceTicket{}, false, nil
	}
	requestID = strings.TrimSpace(requestID)
	state := tracker.state(requestID)
	stale, err := state.acquire(ctx, requestID, appendSeq)
	if err != nil || stale {
		return appendSequenceTicket{}, stale, err
	}
	return appendSequenceTicket{
		state: state,
		seq:   appendSeq,
	}, false, nil
}

func (tracker *appendSequenceTracker) state(requestID string) *appendSequenceState {
	now := time.Now().UTC()
	cutoff := now.Add(-appendSequenceRetention)

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	for key, state := range tracker.states {
		if state == nil || state.expired(cutoff) {
			delete(tracker.states, key)
		}
	}
	if state, ok := tracker.states[requestID]; ok && state != nil {
		state.touch(now)
		return state
	}
	state := &appendSequenceState{
		next:      1,
		ready:     make(chan struct{}),
		updatedAt: now,
	}
	tracker.states[requestID] = state
	return state
}

// acquire 按请求序列协调消息处理；当 Cursor 复用 request_id 并从序号 1 重启时重置空闲状态。
// 参数 ctx 用于取消等待，requestID 用于诊断，appendSeq 表示当前消息序号；返回是否过期及等待错误。
// lyh用cursor修改 2026-08-01：识别同一 request_id 的新轮次序列重启，避免 grep/read 工具结果被持续误判为过期
func (state *appendSequenceState) acquire(ctx context.Context, requestID string, appendSeq int64) (bool, error) {
	for {
		state.mu.Lock()
		now := time.Now().UTC()
		if state.next <= 0 {
			state.next = 1
		}
		if state.ready == nil {
			state.ready = make(chan struct{})
		}
		state.updatedAt = now

		// Cursor may reuse the same request_id for a later turn and restart
		// append_seqno from 1. Accept that as a sequence restart when idle so
		// tool results are not discarded as stale forever.
		if appendSeq == 1 && state.next > 1 {
			if state.processing {
				ready := state.ready
				state.mu.Unlock()
				select {
				case <-ctx.Done():
					return false, ctx.Err()
				case <-ready:
				}
				continue
			}
			prevNext := state.next
			state.next = 1
			state.processing = true
			state.mu.Unlock()
			log.Printf("forwarder reset append sequence request_id=%s previous_next=%d append_seqno=1", requestID, prevNext)
			return false, nil
		}

		switch {
		case appendSeq < state.next:
			state.mu.Unlock()
			return true, nil
		case appendSeq == state.next && !state.processing:
			state.processing = true
			state.mu.Unlock()
			return false, nil
		default:
			ready := state.ready
			state.mu.Unlock()
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-ready:
			}
		}
	}
}

func (state *appendSequenceState) Release(seq int64) {
	if state == nil || seq <= 0 {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.processing && state.next == seq {
		state.processing = false
		state.next++
		close(state.ready)
		state.ready = make(chan struct{})
	}
	state.updatedAt = time.Now().UTC()
}

func (state *appendSequenceState) expired(cutoff time.Time) bool {
	if state == nil {
		return true
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.processing {
		return false
	}
	return !state.updatedAt.IsZero() && state.updatedAt.Before(cutoff)
}

func (state *appendSequenceState) touch(now time.Time) {
	if state == nil {
		return
	}
	state.mu.Lock()
	state.updatedAt = now
	state.mu.Unlock()
}

func (ticket appendSequenceTicket) Release() {
	if ticket.state == nil || ticket.seq <= 0 {
		return
	}
	ticket.state.Release(ticket.seq)
}
