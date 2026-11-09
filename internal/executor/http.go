package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type HTTP struct{}

func (HTTP) Execute(ctx context.Context, ec Context) Result {
	started := time.Now()
	config := mapValue(ec.Target["config"])
	executorConfig := mapValue(ec.Target["executor"])
	baseURL, _ := config["base_url"].(string)
	baseURL = strings.TrimRight(baseURL, "/")
	url, _ := executorConfig["url"].(string)
	if url == "" {
		path, _ := executorConfig["path"].(string)
		url = baseURL + path
	}
	method, _ := executorConfig["method"].(string)
	method = strings.ToUpper(method)
	if method == "" {
		method = "GET"
	}
	var body io.Reader
	if value, exists := executorConfig["body"]; exists && value != nil {
		data, err := json.Marshal(value)
		if err != nil {
			return failedResult(started, "UNKNOWN", err.Error())
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return failedResult(started, "CONNECTION", err.Error())
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range mapValue(executorConfig["headers"]) {
		request.Header.Set(fmt.Sprint(key), fmt.Sprint(value))
	}
	if token, _ := ec.Credential["token"].(string); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	} else if username, exists := ec.Credential["username"]; exists && username != nil {
		request.SetBasicAuth(fmt.Sprint(username), fmt.Sprint(ec.Credential["password"]))
	}
	follow, _ := executorConfig["follow_redirects"].(bool)
	client := &http.Client{Timeout: time.Duration(ec.TimeoutSeconds) * time.Second}
	if !follow {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	response, err := client.Do(request)
	if err != nil {
		duration := int(time.Since(started).Milliseconds())
		if strings.Contains(strings.ToLower(err.Error()), "timeout") {
			class := "TIMEOUT"
			message := "http request timed out"
			return Result{Status: "timed_out", ErrorClass: &class, ErrorMessage: &message, DurationMS: duration}
		}
		class := "CONNECTION"
		message := err.Error()
		if request.Context().Err() == context.Canceled {
			class = "CANCELLED"
		}
		return Result{Status: "failed", ErrorClass: &class, ErrorMessage: &message, DurationMS: duration}
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, int64(ec.OutputLimitBytes)+1))
	if readErr != nil {
		return failedResult(started, "UNKNOWN", readErr.Error())
	}
	excerpt, truncated := truncateBytes(data, ec.OutputLimitBytes)
	duration := int(time.Since(started).Milliseconds())
	successCodes := map[int]bool{}
	if list, ok := executorConfig["success_status_codes"].([]any); ok && len(list) > 0 {
		for _, item := range list {
			if code, ok := toInt(item); ok {
				successCodes[code] = true
			}
		}
	} else {
		for code := 200; code < 300; code++ {
			successCodes[code] = true
		}
	}
	success := successCodes[response.StatusCode]
	if success {
		if pattern, _ := executorConfig["response_body_regex"].(string); pattern != "" {
			matched, err := regexp.MatchString(pattern, excerpt)
			if err != nil || !matched {
				success = false
			}
		}
	}
	if success {
		if headers, ok := executorConfig["response_header_regex"].(map[string]any); ok {
			for name, pattern := range headers {
				matched, err := regexp.MatchString(fmt.Sprint(pattern), response.Header.Get(name))
				if err != nil || !matched {
					success = false
					break
				}
			}
		}
	}
	status := response.StatusCode
	if success {
		return Result{Status: "succeeded", HTTPStatus: &status, Response: &excerpt,
			DurationMS: duration, Truncated: truncated}
	}
	class := "RESPONSE_MISMATCH"
	if status >= 400 && status < 500 {
		class = "HTTP_4XX"
	} else if status >= 500 {
		class = "HTTP_5XX"
	}
	// Preserve the current Python behavior: a non-success response is a
	// response mismatch, because every status-based success path returned above.
	class = "RESPONSE_MISMATCH"
	message := fmt.Sprintf("unexpected HTTP status %d", status)
	return Result{Status: "failed", ErrorClass: &class, ErrorMessage: &message,
		HTTPStatus: &status, Response: &excerpt, DurationMS: duration, Truncated: truncated}
}

func toInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := strconv.Atoi(v.String())
		return i, err == nil
	case string:
		i, err := strconv.Atoi(v)
		return i, err == nil
	default:
		return 0, false
	}
}
