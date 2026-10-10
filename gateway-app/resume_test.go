package main

import (
	"context"
	"testing"
	"time"
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
	_ = trackAttach(ctx, "smoke", func() error { return nil })
	if _, ok := s.recent("k"); ok {
		t.Fatal("a detach with the client still connected should clear the entry")
	}
}

func TestTrackAttachDropRemembers(t *testing.T) {
	s := newResumeStore()
	ctx, cancel := context.WithCancel(context.Background())
	ctx = withResumeTracker(ctx, &resumeTracker{store: s, key: "k"})
	_ = trackAttach(ctx, "smoke", func() error { cancel(); return context.Canceled })
	if a, ok := s.recent("k"); !ok || a != "smoke" {
		t.Fatalf("recent = %q, %v; want smoke, true after a dropped connection", a, ok)
	}
}
