package main

import (
	"context"
	"sync"
	"time"
)

// resumeWindow is how long after a dropped attach a reconnect goes straight
// back into the same agent instead of showing the menu.
const resumeWindow = 15 * time.Minute

// resumeStore remembers, per (public key, username), the agent a session was
// attached to when its connection dropped. Phone clients like Moshi
// disconnect after a short idle; their reconnect should land back in the
// agent, while a fresh login (or a deliberate detach) gets the menu. It is
// in memory only: a gateway restart forgets everything, which is fine.
type resumeStore struct {
	mu      sync.Mutex
	entries map[string]resumeEntry
	now     func() time.Time
}

type resumeEntry struct {
	agent string
	at    time.Time
}

func newResumeStore() *resumeStore {
	return &resumeStore{entries: map[string]resumeEntry{}, now: time.Now}
}

func (s *resumeStore) remember(key, agent string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = resumeEntry{agent: agent, at: s.now()}
}

func (s *resumeStore) forget(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
}

// recent returns the remembered agent if the drop happened within
// resumeWindow.
func (s *resumeStore) recent(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok {
		return "", false
	}
	if s.now().Sub(e.at) > resumeWindow {
		delete(s.entries, key)
		return "", false
	}
	return e.agent, true
}

type resumeTracker struct {
	store *resumeStore
	key   string
}

type resumeTrackerKey struct{}

func withResumeTracker(ctx context.Context, t *resumeTracker) context.Context {
	return context.WithValue(ctx, resumeTrackerKey{}, t)
}

// trackAttach wraps an attach so the session's tracker (if any) records how
// it ended: if the client connection is gone when the attach returns, the
// drop is remembered for a quick reconnect; if the client is still there,
// the user detached on purpose and the next login gets the menu.
func trackAttach(ctx context.Context, agent string, attach func() error) error {
	t, _ := ctx.Value(resumeTrackerKey{}).(*resumeTracker)
	if t == nil {
		return attach()
	}
	t.store.remember(t.key, agent) // covers a drop that kills us mid-attach
	err := attach()
	if ctx.Err() != nil {
		t.store.remember(t.key, agent) // restart the window from the drop
	} else {
		t.store.forget(t.key)
	}
	return err
}
