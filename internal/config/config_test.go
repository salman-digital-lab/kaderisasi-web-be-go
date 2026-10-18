package config

import "testing"

func TestLoad(t *testing.T) {
	base := map[string]string{"APP_KEY": "synthetic-test-key-never-use", "DB_HOST": "localhost", "DB_USER": "user", "DB_DATABASE": "db", "PORT": "", "WEB_BE_LEGACY_URL": "http://localhost:3335/"}
	set := func(t *testing.T, overrides map[string]string) {
		for k, v := range base {
			t.Setenv(k, v)
		}
		for k, v := range overrides {
			t.Setenv(k, v)
		}
	}
	set(t, nil)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 3333 || c.LegacyURL.String() != "http://localhost:3335" {
		t.Fatalf("unexpected config: port %d legacy %s", c.Port, c.LegacyURL)
	}
	for name, overrides := range map[string]map[string]string{
		"missing legacy":   {"WEB_BE_LEGACY_URL": ""},
		"legacy with path": {"WEB_BE_LEGACY_URL": "http://localhost:3335/v2"},
		"legacy scheme":    {"WEB_BE_LEGACY_URL": "ftp://localhost:3335"},
		"legacy userinfo":  {"WEB_BE_LEGACY_URL": "http://user:pass@localhost:3335"},
		"admin port":       {"PORT": "3334"},
		"invalid port":     {"PORT": "abc"},
		"missing key":      {"APP_KEY": ""},
	} {
		t.Run(name, func(t *testing.T) {
			set(t, overrides)
			if _, err := Load(); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
