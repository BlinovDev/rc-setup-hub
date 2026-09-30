package config

import "testing"

func TestLoad(t *testing.T) {
	for _, tc := range []struct{ name, env, want string }{
		{"default", "", ":8080"},
		{"configured", "127.0.0.1:9090", "127.0.0.1:9090"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HTTP_ADDR", tc.env)
			t.Setenv("DATABASE_URL", "postgres://localhost/test")
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DatabaseURL != "postgres://localhost/test" {
				t.Fatal("DATABASE_URL not loaded")
			}
			if got := cfg.HTTPAddr; got != tc.want {
				t.Fatalf("HTTPAddr = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMissingDatabaseURL(t *testing.T) {
	for _, value := range []string{"", "  "} {
		t.Setenv("DATABASE_URL", value)
		if _, err := Load(); err == nil {
			t.Fatal("expected missing DATABASE_URL error")
		}
	}
}
