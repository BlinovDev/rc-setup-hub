package config

import (
	"reflect"
	"testing"
)

func TestParseAdminEmails(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		want        []string
		invalid     bool
	}{
		{"one", "driver@example.com", []string{"driver@example.com"}, false},
		{"multiple", "one@example.com,two@example.com", []string{"one@example.com", "two@example.com"}, false},
		{"whitespace and case", "  DRIVER@Example.COM , , second@example.com, ", []string{"driver@example.com", "second@example.com"}, false},
		{"empty", "", []string{}, false},
		{"empty items", ", ,", []string{}, false},
		{"malformed", "driver@example.com,broken", nil, true},
		{"display name", "Driver <driver@example.com>", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAdminEmails(tc.input)
			if (err != nil) != tc.invalid || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v %v want %v", got, err, tc.want)
			}
		})
	}
}
func TestAdminEmailsEnvironment(t *testing.T) {
	authEnv(t)
	t.Setenv("ADMIN_EMAILS", " DRIVER@Example.COM, ")
	cfg, err := LoadAuth()
	if err != nil || !reflect.DeepEqual(cfg.AdminEmails, []string{"driver@example.com"}) {
		t.Fatal(cfg.AdminEmails, err)
	}
	t.Setenv("ADMIN_EMAILS", "driver@example.com,broken")
	if _, err := LoadAuth(); err == nil {
		t.Fatal("malformed allowlist accepted")
	}
}
