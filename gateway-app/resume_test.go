package main

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestResumeStoreWindow(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	s := newResumeStore()
	s.now = func() time.Time { return now }

	s.remember("k", "smoke")
	if a, ok := s.recent("k"); !ok || a != "smoke" {
		t.Fatalf("recent = %q, %v; want smoke, true", a, ok)
	}
	now = now.Add(resumeWindow + time.Second)
	if _, ok := s.recent("k"); ok {
		t.Fatal("entry should expire after resumeWindow")
	}
}

func TestTrackAttachDeliberateDetachForgets(t *testing.T) {
	s := newResumeStore()
	ctx := withResumeTracker(context.Background(), &resumeTracker{store: s, key: "k"})
	_ = trackAttach(ctx, "smoke", strings.NewReader("x"), func(in io.Reader) error { _, _ = io.ReadAll(io.LimitReader(in, 1)); return nil })
	if _, ok := s.recent("k"); ok {
		t.Fatal("a detach with the client still connected should clear the entry")
	}
}

func TestTrackAttachDropRemembers(t *testing.T) {
	s := newResumeStore()
	ctx, cancel := context.WithCancel(context.Background())
	ctx = withResumeTracker(ctx, &resumeTracker{store: s, key: "k"})
	_ = trackAttach(ctx, "smoke", strings.NewReader(""), func(io.Reader) error { cancel(); return context.Canceled })
	if a, ok := s.recent("k"); !ok || a != "smoke" {
		t.Fatalf("recent = %q, %v; want smoke, true after a dropped connection", a, ok)
	}
}

func TestTrackAttachStdinEOFRemembers(t *testing.T) {
	s := newResumeStore()
	ctx := withResumeTracker(context.Background(), &resumeTracker{store: s, key: "k"})
	_ = trackAttach(ctx, "smoke", strings.NewReader("abc"), func(in io.Reader) error {
		_, err := io.ReadAll(in) // client closed its side: stdin EOF, context still live
		return err
	})
	if a, ok := s.recent("k"); !ok || a != "smoke" {
		t.Fatalf("recent = %q, %v; want smoke, true after stdin EOF", a, ok)
	}
}

func TestResumePromptKeys(t *testing.T) {
	m := newResumePromptModel("smoke")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if rp := next.(resumePromptModel); !rp.resume || !rp.done {
		t.Errorf("enter should resume now, got %+v", rp)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if rp := next.(resumePromptModel); rp.resume || !rp.done {
		t.Errorf("another key should go to the menu, got %+v", rp)
	}
	for i := 0; i < resumeCountdown; i++ {
		next, _ = m.Update(resumeTickMsg{})
		m = next.(resumePromptModel)
	}
	if !m.resume || !m.done {
		t.Errorf("countdown expiry should resume, got %+v", m)
	}
}
