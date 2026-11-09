package core

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tfenng/tasks/task2go/internal/params"
	"github.com/tfenng/tasks/task2go/internal/security"
	"github.com/tfenng/tasks/task2go/internal/store"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrTargetDisabled = errors.New("target disabled")
	ErrValidation     = errors.New("validation error")
	ErrConflict       = errors.New("conflict")
)

type RetryPolicy struct {
	MaxAttempts           int
	InitialBackoffSeconds int
	Multiplier            float64
	MaxBackoffSeconds     int
	RetryOnExitCodes      []int
	RetryableErrorClasses map[string]bool
}

func ParseRetryPolicy(raw map[string]any) RetryPolicy {
	p := RetryPolicy{
		MaxAttempts:           1,
		InitialBackoffSeconds: 30,
		Multiplier:            2,
		MaxBackoffSeconds:     600,
		RetryableErrorClasses: map[string]bool{
			"TIMEOUT": true, "CONNECTION": true, "HTTP_429": true,
			"HTTP_5XX": true, "WORKER_LOST": true,
		},
	}
	if raw == nil {
		return p
	}
	if v, ok := intValue(raw["max_attempts"]); ok {
		p.MaxAttempts = v
	}
	if v, ok := intValue(raw["initial_backoff_seconds"]); ok {
		p.InitialBackoffSeconds = v
	}
	if v, ok := floatValue(raw["multiplier"]); ok {
		p.Multiplier = v
	}
	if v, ok := intValue(raw["max_backoff_seconds"]); ok {
		p.MaxBackoffSeconds = v
	}
	if list, ok := raw["retry_on_exit_codes"].([]any); ok {
		for _, item := range list {
			if v, ok := intValue(item); ok {
				p.RetryOnExitCodes = append(p.RetryOnExitCodes, v)
			}
		}
	}
	if list, ok := raw["retryable_error_classes"].([]any); ok {
		p.RetryableErrorClasses = map[string]bool{}
		for _, item := range list {
			if s, ok := item.(string); ok {
				p.RetryableErrorClasses[s] = true
			}
		}
	}
	return p
}

func (p RetryPolicy) Retryable(errorClass string, exitCode *int) bool {
	if p.RetryableErrorClasses[errorClass] {
		return true
	}
	if errorClass == "EXIT_CODE" && exitCode != nil {
		for _, code := range p.RetryOnExitCodes {
			if code == *exitCode {
				return true
			}
		}
	}
	return false
}

func (p RetryPolicy) Delay(attemptCount int) time.Duration {
	power := attemptCount - 1
	if power < 0 {
		power = 0
	}
	delay := float64(p.InitialBackoffSeconds)
	for i := 0; i < power; i++ {
		delay *= p.Multiplier
	}
	if delay > float64(p.MaxBackoffSeconds) {
		delay = float64(p.MaxBackoffSeconds)
	}
	return time.Duration(delay) * time.Second
}

func ValidateExecutorConfig(executorType string, config map[string]any) error {
	if config == nil {
		return fmt.Errorf("%w: executor_config must be an object", ErrValidation)
	}
	if executorType == "local" {
		argv, ok := config["argv"].([]any)
		if !ok || len(argv) == 0 {
			return fmt.Errorf("%w: local executor requires a non-empty argv string array", ErrValidation)
		}
		for _, item := range argv {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("%w: local executor requires a non-empty argv string array", ErrValidation)
			}
		}
		if env, exists := config["env"]; exists {
			if _, ok := env.(map[string]any); !ok {
				return fmt.Errorf("%w: executor_config.env must be an object", ErrValidation)
			}
		}
		if cwd, exists := config["cwd"]; exists {
			if _, ok := cwd.(string); !ok {
				return fmt.Errorf("%w: executor_config.cwd must be a string", ErrValidation)
			}
		}
		return nil
	}
	if s, _ := config["target_id"].(string); s == "" {
		return fmt.Errorf("%w: %s executor requires target_id", ErrValidation, executorType)
	}
	if executorType == "ssh" {
		if s, _ := config["command"].(string); s == "" {
			return fmt.Errorf("%w: ssh executor requires command", ErrValidation)
		}
	}
	if executorType == "http" {
		path, _ := config["path"].(string)
		url, _ := config["url"].(string)
		if path == "" && url == "" {
			return fmt.Errorf("%w: http executor requires path or url", ErrValidation)
		}
		method, _ := config["method"].(string)
		if method == "" {
			method = "GET"
		}
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD":
		default:
			return fmt.Errorf("%w: unsupported http method", ErrValidation)
		}
	}
	return nil
}

func Checksum(payload map[string]any) string {
	data, _ := params.CanonicalJSON(payload)
	return params.SumSHA256(data)
}

func RequestHash(definitionID string, arguments []string, parameters map[string]any) string {
	payload := map[string]any{
		"definition_id": definitionID,
		"arguments":     stringSliceToAny(arguments),
		"parameters":    parameters,
	}
	return Checksum(payload)
}

func ComputeNextFire(triggerType string, fireAt *time.Time, cronExpr *string, intervalSeconds *int, tzName string, after time.Time) (*time.Time, error) {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid timezone: %s", ErrValidation, tzName)
	}
	after = after.UTC()
	switch triggerType {
	case "once":
		if fireAt != nil && fireAt.After(after) {
			v := fireAt.UTC()
			return &v, nil
		}
		return nil, nil
	case "interval":
		if intervalSeconds == nil || *intervalSeconds < 10 {
			return nil, fmt.Errorf("%w: interval must be at least 10 seconds", ErrValidation)
		}
		v := after.Add(time.Duration(*intervalSeconds) * time.Second)
		return &v, nil
	case "cron":
		if cronExpr == nil || *cronExpr == "" {
			return nil, fmt.Errorf("%w: cron expression is required", ErrValidation)
		}
		parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		sched, err := parser.Parse(*cronExpr)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid cron expression", ErrValidation)
		}
		next := sched.Next(after.In(loc))
		v := next.UTC()
		return &v, nil
	default:
		return nil, fmt.Errorf("%w: unsupported trigger type", ErrValidation)
	}
}

func BuildTargetSnapshot(ctx context.Context, s *store.Store, q store.Querier, executorConfig map[string]any) (map[string]any, error) {
	targetID, _ := executorConfig["target_id"].(string)
	if targetID == "" {
		return executorConfig, nil
	}
	target, err := s.GetTarget(ctx, q, targetID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("%w: target not found", ErrNotFound)
	}
	if !target.IsActive {
		return nil, ErrTargetDisabled
	}
	executor := map[string]any{}
	for k, v := range executorConfig {
		if k != "target_id" {
			executor[k] = v
		}
	}
	snapshot := map[string]any{
		"type":      target.Type,
		"target_id": target.ID,
		"name":      target.Name,
		"config":    target.Config.Map(),
		"executor":  executor,
	}
	if target.CredentialID != nil {
		snapshot["credential_id"] = *target.CredentialID
	}
	return snapshot, nil
}

func CredentialValues(ctx context.Context, s *store.Store, q store.Querier, box *security.SecretBox, credentialID string) (map[string]any, error) {
	credential, err := s.GetCredential(ctx, q, credentialID)
	if err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, fmt.Errorf("%w: credential not found", ErrNotFound)
	}
	if !credential.IsActive {
		return nil, ErrTargetDisabled
	}
	var payload map[string]any
	if err := jsonUnmarshalNumber(credential.Ciphertext, &payload); err != nil {
		return nil, err
	}
	secret, err := box.Decrypt(payload, fmt.Sprintf("credential:%s:%s", credential.ID, credential.Kind))
	if err != nil {
		return nil, err
	}
	switch credential.Kind {
	case "password":
		return map[string]any{"password": secret}, nil
	case "ssh_key":
		return map[string]any{"private_key": secret, "passphrase": metadataString(credential.Metadata, "passphrase")}, nil
	case "http_bearer":
		return map[string]any{"token": secret}, nil
	case "http_basic":
		return map[string]any{"username": metadataString(credential.Metadata, "username"), "password": secret}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported credential kind", ErrValidation)
	}
}

func EncryptCredential(box *security.SecretBox, credentialID, kind, secret string) ([]byte, int, error) {
	payload, err := box.Encrypt(secret, fmt.Sprintf("credential:%s:%s", credentialID, kind))
	if err != nil {
		return nil, 0, err
	}
	data, err := params.CanonicalJSON(payload)
	if err != nil {
		return nil, 0, err
	}
	return data, box.ActiveVersion, nil
}

func metadataString(metadata store.JSONB, key string) any {
	if metadata == nil {
		return nil
	}
	return metadata.Map()[key]
}

func intValue(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	default:
		if v == nil {
			return 0, false
		}
		// json.Number has a String method; use a type assertion without importing encoding/json here.
		if s, ok := v.(fmt.Stringer); ok {
			i, err := strconv.Atoi(s.String())
			return i, err == nil
		}
		return 0, false
	}
}

func floatValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		if v == nil {
			return 0, false
		}
		if s, ok := v.(fmt.Stringer); ok {
			f, err := strconv.ParseFloat(s.String(), 64)
			return f, err == nil
		}
		return 0, false
	}
}

func stringSliceToAny(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}
