// Package config reads the web API environment. It uses the same variable
// names as the AdonisJS web-be service (env/<environment>/web-be-*-env), plus
// WEB_BE_LEGACY_URL: the Adonis service that still answers every route the Go
// service does not own yet.
package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment string
	Host        string
	Port        int
	AppKey      string
	LogLevel    string
	DBHost      string
	DBPort      uint16
	DBUser      string
	DBPassword  string
	DBName      string
	DBSchema    string
	Location    *time.Location
	LegacyURL   *url.URL
}

func Load() (Config, error) {
	port, err := strconv.Atoi(value("PORT", "3333"))
	if err != nil || port < 1 || port > 65535 {
		return Config{}, errors.New("invalid PORT")
	}
	if port == 3334 {
		return Config{}, errors.New("PORT 3334 belongs to the admin API")
	}
	dbPort, err := strconv.ParseUint(value("DB_PORT", "5432"), 10, 16)
	if err != nil {
		return Config{}, errors.New("invalid DB_PORT")
	}
	schema := value("DB_SCHEMA", "public")
	if !regexp.MustCompile(`^[a-z_][a-z0-9_]*$`).MatchString(schema) {
		return Config{}, errors.New("invalid DB_SCHEMA")
	}
	location := time.Local
	if tz := os.Getenv("TZ"); tz != "" {
		location, err = time.LoadLocation(tz)
		if err != nil {
			return Config{}, errors.New("invalid TZ")
		}
	}
	c := Config{Environment: value("NODE_ENV", "development"), Host: value("HOST", "127.0.0.1"), Port: port,
		AppKey: os.Getenv("APP_KEY"), LogLevel: value("LOG_LEVEL", "info"), DBHost: os.Getenv("DB_HOST"),
		DBPort: uint16(dbPort), DBUser: os.Getenv("DB_USER"), DBPassword: os.Getenv("DB_PASSWORD"), DBName: os.Getenv("DB_DATABASE"),
		DBSchema: schema, Location: location}
	if len(c.AppKey) < 16 || c.DBHost == "" || c.DBUser == "" || c.DBName == "" {
		return Config{}, errors.New("APP_KEY and database configuration are required")
	}
	raw := strings.TrimRight(strings.TrimSpace(os.Getenv("WEB_BE_LEGACY_URL")), "/")
	if raw == "" {
		return Config{}, errors.New("WEB_BE_LEGACY_URL is required while web-be routes are proxied")
	}
	legacy, err := url.Parse(raw)
	if err != nil || (legacy.Scheme != "http" && legacy.Scheme != "https") || legacy.Hostname() == "" || legacy.User != nil || legacy.Path != "" || legacy.RawQuery != "" || legacy.Fragment != "" {
		return Config{}, errors.New("invalid WEB_BE_LEGACY_URL")
	}
	c.LegacyURL = legacy
	return c, nil
}

func (c Config) Address() string { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
