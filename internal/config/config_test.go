package config

import "testing"

func TestLoad(t *testing.T) {
	base := map[string]string{"APP_KEY": "synthetic-test-key-never-use", "DB_HOST": "localhost", "DB_USER": "user", "DB_DATABASE": "db", "PORT": "", "RESET_PASSWORD_URL": "http://localhost:3000/reset", "MAIL_FROM": "noreply@example.test", "SMTP_HOST": "localhost", "SMTP_PORT": "2525"}
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
	if c.Port != 3333 {
		t.Fatalf("unexpected config: port %d", c.Port)
	}
	for name, overrides := range map[string]map[string]string{
		"admin port":   {"PORT": "3334"},
		"invalid port": {"PORT": "abc"},
		"missing key":  {"APP_KEY": ""},
		"missing smtp": {"SMTP_HOST": ""},
	} {
		t.Run(name, func(t *testing.T) {
			set(t, overrides)
			if _, err := Load(); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
