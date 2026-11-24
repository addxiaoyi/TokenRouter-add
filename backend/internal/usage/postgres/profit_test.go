package postgres

import "testing"

// profit_test.go protects website hints returned with upstream cost reminders.
// Report links retain only the public origin, so credentials embedded in older
// provider URL settings cannot leak into analytics or an administrator's clipboard.

// TestProfitSite strips authentication and query material from valid websites.
// Unsupported schemes and relative paths must never become clickable report links.
func TestProfitSite(t *testing.T) {
	for _, test := range []struct {
		input string // raw configured URL
		want  string // safe website origin
	}{
		{"https://user:secret@example.com:8443/v1?key=secret#token", "https://example.com:8443"},
		{"https://example.com/v1", "https://example.com"},
		{"javascript:alert(1)", ""},
		{"/v1", ""},
	} {
		if got := profitSite(test.input); got != test.want {
			t.Fatalf("profitSite() = %q, want %q", got, test.want)
		}
	}
}
