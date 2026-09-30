package config

import "testing"

func TestAppURL(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"http://localhost:5173", "http://localhost:5173"},
		{" https://APP.example.com/welcome?source=login#home ", "https://app.example.com/welcome?source=login#home"},
		{"", ""}, {"/relative", ""}, {"https://", ""}, {"http://localhost:bad", ""},
		{"http://localhost:", ""}, {"http://localhost:70000", ""}, {"http://localhost:0", ""}, {"https://user:password@app.example.com", ""}, {"javascript:alert(1)", ""},
		{"http://localhost:5173/%zz", ""}, {"http://*.example.com", ""}, {"http://localhost\n.evil", ""},
	} {
		t.Run(tc.value, func(t *testing.T) {
			authEnv(t)
			t.Setenv("APP_URL", tc.value)
			cfg, err := LoadAuth()
			if tc.want == "" {
				if err == nil {
					t.Fatal("invalid APP_URL accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.AppURL.String(); got != tc.want {
				t.Fatalf("APP_URL = %q, want %q", got, tc.want)
			}
		})
	}
}
