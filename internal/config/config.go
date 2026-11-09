package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL  string
	DatabaseEcho bool

	SecretKeys          map[int][]byte
	SecretActiveVersion int

	CookieSecure         bool
	SessionCookieName    string
	CSRFCookieName       string
	SessionIdleMinutes   int
	SessionAbsoluteHours int
	SessionRememberDays  int

	ScriptRoot              string
	DefaultShell            string
	KnownHostsPath          string
	OutputLimitBytes        int
	OutputRoot              string
	OutputCaptureLimitBytes int
	LiveOutputMaxBytes      int
	RetentionDays           int
	AttemptRetentionDays    int

	WorkerPollSeconds float64
	WorkerConcurrency int
	LeaseSeconds      int

	AllowedHosts string
	LogLevel     string
	CORSOrigins  string

	HTTPAddr string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	c := &Config{
		DatabaseURL:             env("TASK_CONTROL_DATABASE_URL", "postgresql+psycopg://localhost:5432/task_control"),
		DatabaseEcho:            envBool("TASK_CONTROL_DATABASE_ECHO", false),
		SecretActiveVersion:     envInt("TASK_CONTROL_SECRET_ACTIVE_VERSION", 1),
		CookieSecure:            envBool("TASK_CONTROL_COOKIE_SECURE", false),
		SessionCookieName:       env("TASK_CONTROL_SESSION_COOKIE_NAME", "tc_session"),
		CSRFCookieName:          env("TASK_CONTROL_CSRF_COOKIE_NAME", "tc_csrf"),
		SessionIdleMinutes:      envInt("TASK_CONTROL_SESSION_IDLE_MINUTES", 30),
		SessionAbsoluteHours:    envInt("TASK_CONTROL_SESSION_ABSOLUTE_HOURS", 8),
		SessionRememberDays:     envInt("TASK_CONTROL_SESSION_REMEMBER_DAYS", 7),
		ScriptRoot:              env("TASK_CONTROL_SCRIPT_ROOT", "/tmp/task-control-scripts"),
		DefaultShell:            env("TASK_CONTROL_DEFAULT_SHELL", "/bin/bash"),
		KnownHostsPath:          env("TASK_CONTROL_KNOWN_HOSTS_PATH", ""),
		OutputLimitBytes:        envInt("TASK_CONTROL_OUTPUT_LIMIT_BYTES", 1024*1024),
		OutputRoot:              env("TASK_CONTROL_OUTPUT_ROOT", "/tmp/task-control-output"),
		OutputCaptureLimitBytes: envInt("TASK_CONTROL_OUTPUT_CAPTURE_LIMIT_BYTES", 64*1024*1024),
		LiveOutputMaxBytes:      envInt("TASK_CONTROL_LIVE_OUTPUT_MAX_BYTES", 256*1024),
		RetentionDays:           envInt("TASK_CONTROL_RETENTION_DAYS", 395),
		AttemptRetentionDays:    envInt("TASK_CONTROL_ATTEMPT_RETENTION_DAYS", 90),
		WorkerPollSeconds:       envFloat("TASK_CONTROL_WORKER_POLL_SECONDS", 1.0),
		WorkerConcurrency:       envInt("TASK_CONTROL_WORKER_CONCURRENCY", 4),
		LeaseSeconds:            envInt("TASK_CONTROL_LEASE_SECONDS", 60),
		AllowedHosts:            env("TASK_CONTROL_ALLOWED_HOSTS", "localhost,127.0.0.1,testserver"),
		LogLevel:                env("TASK_CONTROL_LOG_LEVEL", "INFO"),
		CORSOrigins:             env("TASK_CONTROL_CORS_ORIGINS", "http://localhost:5173"),
		HTTPAddr:                env("TASK_CONTROL_HTTP_ADDR", "0.0.0.0:8000"),
	}
	if c.WorkerConcurrency < 1 {
		c.WorkerConcurrency = 1
	}
	if c.OutputLimitBytes < 1 {
		c.OutputLimitBytes = 1
	}
	if c.OutputCaptureLimitBytes < 1 {
		c.OutputCaptureLimitBytes = 1
	}

	raw := env("TASK_CONTROL_SECRET_KEYS", `{"1":"ZGV2LW9ubHktMzItYnl0ZS1rZXktZm9yLXRlc3RzISE="}`)
	var keys map[string]string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil || len(keys) == 0 {
		return nil, fmt.Errorf("TASK_CONTROL_SECRET_KEYS must be a non-empty JSON object")
	}
	c.SecretKeys = make(map[int][]byte, len(keys))
	for version, encoded := range keys {
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("secret version %s is not valid base64", version)
		}
		if len(raw) != 32 {
			return nil, fmt.Errorf("secret version %s must decode to 32 bytes", version)
		}
		n, err := strconv.Atoi(version)
		if err != nil {
			return nil, fmt.Errorf("secret version %s is not an integer", version)
		}
		c.SecretKeys[n] = raw
	}
	if _, ok := c.SecretKeys[c.SecretActiveVersion]; !ok {
		return nil, fmt.Errorf("TASK_CONTROL_SECRET_ACTIVE_VERSION %d has no key", c.SecretActiveVersion)
	}

	c.DatabaseURL = normalizeDatabaseURL(c.DatabaseURL)
	return c, nil
}

func normalizeDatabaseURL(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Replace(value, "postgresql+psycopg://", "postgresql://", 1)
	value = strings.Replace(value, "postgresql+asyncpg://", "postgresql://", 1)
	return value
}

func (c *Config) CORSOriginsList() []string {
	var out []string
	for _, item := range strings.Split(c.CORSOrigins, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func env(key, def string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return def
}

func envBool(key string, def bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envInt(key string, def int) int {
	value, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return def
	}
	return n
}

func envFloat(key string, def float64) float64 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return def
	}
	return n
}
