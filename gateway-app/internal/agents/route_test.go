package agents

import "testing"

func TestDecideRoute(t *testing.T) {
	cases := []struct {
		username  string
		wantKind  RouteKind
		wantAgent string
	}{
		{"levi", RouteMenu, ""},
		{"menu", RouteMenu, ""},
		{"new", RouteNew, ""},
		{"smoke", RouteDirect, "smoke"},
		{"my-agent-1", RouteDirect, "my-agent-1"},
		{"", RouteInvalid, ""},
		{"Bad_Name", RouteInvalid, ""},
		{"-bad", RouteInvalid, ""},
	}
	for _, c := range cases {
		r := DecideRoute(c.username)
		if r.Kind != c.wantKind {
			t.Errorf("DecideRoute(%q).Kind = %v, want %v", c.username, r.Kind, c.wantKind)
		}
		if r.Kind == RouteDirect && r.AgentName != c.wantAgent {
			t.Errorf("DecideRoute(%q).AgentName = %q, want %q", c.username, r.AgentName, c.wantAgent)
		}
		if r.Kind == RouteInvalid && r.Err == nil {
			t.Errorf("DecideRoute(%q) RouteInvalid with nil Err", c.username)
		}
	}
}

func TestRouteKindString(t *testing.T) {
	cases := map[RouteKind]string{
		RouteMenu:    "menu",
		RouteNew:     "new",
		RouteDirect:  "direct",
		RouteInvalid: "invalid",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("RouteKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}
