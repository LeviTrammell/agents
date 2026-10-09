package agents

import "testing"

func TestAllowForwardDestination(t *testing.T) {
	allowed := []string{"localhost", "127.0.0.1", "::1"}
	for _, h := range allowed {
		if !AllowForwardDestination(h) {
			t.Errorf("AllowForwardDestination(%q) = false, want true", h)
		}
	}
	denied := []string{"10.0.0.5", "example.com", "0.0.0.0", "8.8.8.8", "", "localhost.evil.com"}
	for _, h := range denied {
		if AllowForwardDestination(h) {
			t.Errorf("AllowForwardDestination(%q) = true, want false", h)
		}
	}
}

func TestAllowForwardUser(t *testing.T) {
	allowed := []string{"smoke", "my-agent"}
	for _, u := range allowed {
		if !AllowForwardUser(u) {
			t.Errorf("AllowForwardUser(%q) = false, want true", u)
		}
	}
	denied := []string{"levi", "menu", "new", "", "Bad_Name"}
	for _, u := range denied {
		if AllowForwardUser(u) {
			t.Errorf("AllowForwardUser(%q) = true, want false", u)
		}
	}
}
