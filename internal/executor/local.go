package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Local struct{}

func (Local) Execute(_ context.Context, ec Context) Result {
	started := time.Now()
	config := mapValue(ec.Target["executor"])
	if len(config) == 0 {
		config = ec.Target
	}
	rawArgv, _ := config["argv"].([]any)
	argv := make([]string, 0, len(rawArgv)+4)
	for _, item := range rawArgv {
		rendered, err := render(fmt.Sprint(item), ec.Parameters)
		if err != nil {
			return failedResult(started, "UNKNOWN", err.Error())
		}
		argv = append(argv, rendered)
	}
	if interpreter, _ := config["interpreter"].(string); interpreter != "" {
		argv = append([]string{interpreter}, argv...)
	} else {
		argv = applyShebang(argv, ec.DefaultShell)
	}
	if extra, ok := ec.Parameters["__arguments__"].([]any); ok {
		for _, item := range extra {
			argv = append(argv, fmt.Sprint(item))
		}
	}
	if len(argv) == 0 {
		return failedResult(started, "UNKNOWN", "empty argv")
	}
	dir, err := EnsureAttemptDir(ec.OutputRoot, ec.RunID, ec.AttemptNo)
	if err != nil {
		return failedResult(started, "UNKNOWN", err.Error())
	}
	stdoutPath := filepath.Join(dir, StdoutName)
	stderrPath := filepath.Join(dir, StderrName)

	cmd := exec.Command(argv[0], argv[1:]...)
	if cwd, _ := config["cwd"].(string); cwd != "" {
		cmd.Dir = cwd
	}
	env := os.Environ()
	env = append(env, "TASK_RUN_ID="+ec.RunID)
	for key, value := range mapValue(config["env"]) {
		env = append(env, fmt.Sprint(key)+"="+fmt.Sprint(value))
	}
	for key, value := range ec.Parameters {
		env = append(env, "TASK_PARAM_"+strings.ToUpper(fmt.Sprint(key))+"="+fmt.Sprint(value))
	}
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return failedResult(started, "UNKNOWN", err.Error())
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return failedResult(started, "UNKNOWN", err.Error())
	}
	if err := cmd.Start(); err != nil {
		return failedResult(started, "UNKNOWN", err.Error())
	}
	var wg sync.WaitGroup
	var outTruncated, errTruncated bool
	var outErr, errErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, outTruncated, outErr = Pump(stdoutPipe, stdoutPath, ec.OutputCaptureLimitBytes, ec.SecretValues)
	}()
	go func() {
		defer wg.Done()
		_, errTruncated, errErr = Pump(stderrPipe, stderrPath, ec.OutputCaptureLimitBytes, ec.SecretValues)
	}()
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	timedOut := false
	select {
	case <-time.After(time.Duration(ec.TimeoutSeconds) * time.Second):
		timedOut = true
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-waitDone:
		case <-time.After(10 * time.Second):
		}
	case <-waitDone:
	}
	wg.Wait()
	if !timedOut && (outErr != nil || errErr != nil) {
		return failedResult(started, "UNKNOWN", "read process output failed")
	}
	stdout, outTotal, outCut := ReadTail(stdoutPath, ec.OutputLimitBytes)
	stderr, errTotal, errCut := ReadTail(stderrPath, ec.OutputLimitBytes)
	truncated := outTruncated || errTruncated || outCut || errCut ||
		outTotal > ec.OutputLimitBytes || errTotal > ec.OutputLimitBytes
	stdout = redact(stdout, ec.SecretValues)
	stderr = redact(stderr, ec.SecretValues)
	duration := int(time.Since(started).Milliseconds())
	if timedOut {
		class := "TIMEOUT"
		message := fmt.Sprintf("execution timed out after %ds", ec.TimeoutSeconds)
		return Result{
			Status: "timed_out", ErrorClass: &class, ErrorMessage: &message,
			Stdout: &stdout, Stderr: &stderr, DurationMS: duration, Truncated: truncated,
		}
	}
	code := cmd.ProcessState.ExitCode()
	if code == 0 {
		return Result{
			Status: "succeeded", ExitCode: &code, Stdout: &stdout, Stderr: &stderr,
			DurationMS: duration, Truncated: truncated,
		}
	}
	class := "EXIT_CODE"
	message := fmt.Sprintf("process exited with code %d", code)
	return Result{
		Status: "failed", ErrorClass: &class, ErrorMessage: &message, ExitCode: &code,
		Stdout: &stdout, Stderr: &stderr, DurationMS: duration, Truncated: truncated,
	}
}

func applyShebang(argv []string, defaultShell string) []string {
	if len(argv) != 1 {
		return argv
	}
	info, err := os.Stat(argv[0])
	if err != nil || info.IsDir() {
		return argv
	}
	file, err := os.Open(argv[0])
	if err == nil {
		first := make([]byte, 512)
		n, _ := file.Read(first)
		_ = file.Close()
		line := string(first[:n])
		if idx := strings.IndexByte(line, '\n'); idx >= 0 {
			line = line[:idx]
		}
		if strings.HasPrefix(line, "#!") {
			if parts := splitShebang(strings.TrimSpace(line[2:])); len(parts) > 0 {
				return append(parts, argv[0])
			}
		}
	}
	if strings.EqualFold(filepath.Ext(argv[0]), ".py") {
		return []string{"python3", argv[0]}
	}
	return []string{defaultShell, argv[0]}
}

func failedResult(started time.Time, class, message string) Result {
	return Result{
		Status: "failed", ErrorClass: &class, ErrorMessage: &message,
		DurationMS: int(time.Since(started).Milliseconds()),
	}
}
