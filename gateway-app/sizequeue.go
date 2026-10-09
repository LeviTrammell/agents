package main

import (
	"context"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	sshlib "charm.land/ssh"
	"k8s.io/client-go/tools/remotecommand"
)

// sizeBroadcaster is the single consumer of an SSH session's window-change
// channel for the session's whole lifetime. It feeds a latest-value-wins
// remotecommand.TerminalSizeQueue (for whichever pod exec is currently
// running, if any) and, whenever a Bubble Tea program is the foreground UI,
// also forwards the resize to it as a tea.WindowSizeMsg -- the same thing
// charm.land/wish/v2/bubbletea's own middleware does, just reusable across
// several Program runs and an exec in the same connection instead of one
// Program for the whole session.
type sizeBroadcaster struct {
	queue *sizeQueue

	mu      sync.Mutex
	program *tea.Program
}

func newSizeBroadcaster(ctx context.Context, initial sshlib.Window, changes <-chan sshlib.Window) *sizeBroadcaster {
	b := &sizeBroadcaster{queue: newSizeQueue(initial)}
	go b.run(ctx, changes)
	return b
}

func (b *sizeBroadcaster) run(ctx context.Context, changes <-chan sshlib.Window) {
	for {
		select {
		case <-ctx.Done():
			b.queue.close()
			return
		case w, ok := <-changes:
			if !ok {
				b.queue.close()
				return
			}
			b.queue.update(w)
			b.mu.Lock()
			p := b.program
			b.mu.Unlock()
			if p != nil {
				p.Send(tea.WindowSizeMsg{Width: w.Width, Height: w.Height})
			}
		}
	}
}

// setProgram registers the Bubble Tea program that should receive
// WindowSizeMsgs for as long as it's the foreground UI. Pass nil when no
// program is running (e.g. during a direct attach with no menu).
func (b *sizeBroadcaster) setProgram(p *tea.Program) {
	b.mu.Lock()
	b.program = p
	b.mu.Unlock()
}

// TerminalSizeQueue returns the remotecommand.TerminalSizeQueue fed by this
// broadcaster.
func (b *sizeBroadcaster) TerminalSizeQueue() remotecommand.TerminalSizeQueue {
	return b.queue
}

// sizeQueue is a latest-value-wins remotecommand.TerminalSizeQueue: a
// writer that outruns the reader only ever leaves the most recent size
// pending, which is the right behavior for terminal resizes (no point
// replaying every intermediate size).
type sizeQueue struct {
	ch     chan remotecommand.TerminalSize
	closed atomic.Bool
}

func newSizeQueue(initial sshlib.Window) *sizeQueue {
	q := &sizeQueue{ch: make(chan remotecommand.TerminalSize, 1)}
	q.update(initial)
	return q
}

func (q *sizeQueue) update(w sshlib.Window) {
	if q.closed.Load() {
		return
	}
	ts := remotecommand.TerminalSize{Width: uint16(w.Width), Height: uint16(w.Height)}
	for {
		select {
		case q.ch <- ts:
			return
		default:
		}
		select {
		case <-q.ch:
		default:
			return
		}
	}
}

func (q *sizeQueue) close() {
	if q.closed.CompareAndSwap(false, true) {
		close(q.ch)
	}
}

// Next implements remotecommand.TerminalSizeQueue.
func (q *sizeQueue) Next() *remotecommand.TerminalSize {
	ts, ok := <-q.ch
	if !ok {
		return nil
	}
	return &ts
}
