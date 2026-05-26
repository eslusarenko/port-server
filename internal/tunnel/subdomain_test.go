package tunnel

import (
	"regexp"
	"testing"
)

func TestGenerateSubdomain(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-z0-9]{8}$`)

	for i := 0; i < 100; i++ {
		s, err := GenerateSubdomain()
		if err != nil {
			t.Fatalf("GenerateSubdomain: %v", err)
		}
		if !pattern.MatchString(s) {
			t.Errorf("subdomain %q does not match pattern [a-z0-9]{8}", s)
		}
	}
}

func TestGenerateSubdomainUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		s, err := GenerateSubdomain()
		if err != nil {
			t.Fatalf("GenerateSubdomain: %v", err)
		}
		if seen[s] {
			t.Errorf("duplicate subdomain: %q", s)
		}
		seen[s] = true
	}
}

func TestIsReserved(t *testing.T) {
	reserved := []string{"admin", "login", "DASHBOARD"}
	cases := []struct {
		sub  string
		want bool
	}{
		{"admin", true},
		{"ADMIN", true},
		{"Admin", true},
		{"login", true},
		{"dashboard", true},
		{"DASHBOARD", true},
		{"random", false},
		{"adminx", false},
		{"xadmin", false},
	}
	for _, c := range cases {
		got := IsReserved(c.sub, reserved)
		if got != c.want {
			t.Errorf("IsReserved(%q) = %v, want %v", c.sub, got, c.want)
		}
	}
}

func TestIsReservedNilList(t *testing.T) {
	if IsReserved("admin", nil) {
		t.Error("IsReserved with nil list should return false")
	}
}

