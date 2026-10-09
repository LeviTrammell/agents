package agents

import "testing"

func TestValidateName(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"foo", false},
		{"foo-bar", false},
		{"a", false},
		{"a1", false},
		{"123", false},
		{"", true},
		{"-foo", true},
		{"foo-", true},
		{"Foo", true},
		{"foo_bar", true},
		{"foo bar", true},
		{"foo.bar", true},
		// exactly 50 chars: ok
		{repeatChar('a', 50), false},
		// 51 chars: too long
		{repeatChar('a', 51), true},
	}
	for _, c := range cases {
		err := ValidateName(c.name)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateName(%q) error = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}

func repeatChar(c byte, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return string(b)
}

func TestIsReserved(t *testing.T) {
	for _, n := range []string{"levi", "menu", "new"} {
		if !IsReserved(n) {
			t.Errorf("IsReserved(%q) = false, want true", n)
		}
	}
	for _, n := range []string{"smoke", "levii", "Menu", "foo"} {
		if IsReserved(n) {
			t.Errorf("IsReserved(%q) = true, want false", n)
		}
	}
}

func TestSandboxName(t *testing.T) {
	if got, want := SandboxName("smoke"), "agent-smoke"; got != want {
		t.Errorf("SandboxName(smoke) = %q, want %q", got, want)
	}
}
