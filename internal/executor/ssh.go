package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type SSH struct{}

func (SSH) Execute(_ context.Context, ec Context) Result {
	started := time.Now()
	target := ec.Target
	config := mapValue(target["config"])
	executorConfig := mapValue(target["executor"])
	host, _ := config["host"].(string)
	username, _ := config["user"].(string)
	port := 22
	if value, ok := config["port"]; ok {
		if n, ok := toInt(value); ok {
			port = n
		}
	}
	command, _ := executorConfig["command"].(string)
	if extra, ok := ec.Parameters["__arguments__"].([]any); ok && len(extra) > 0 {
		args := make([]string, 0, len(extra))
		for _, item := range extra {
			args = append(args, fmt.Sprint(item))
		}
		command = command + " " + ShellJoin(args)
	}
	credential := ec.Credential
	auth := []ssh.AuthMethod{}
	if key, _ := credential["private_key"].(string); key != "" {
		passphrase, _ := credential["passphrase"].(string)
		var signer ssh.Signer
		var err error
		if passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(key), []byte(passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(key))
		}
		if err != nil {
			return failedResult(started, "SSH_AUTH", err.Error())
		}
		auth = append(auth, ssh.PublicKeys(signer))
	} else if password, _ := credential["password"].(string); password != "" {
		auth = append(auth, ssh.Password(password))
	} else {
		return failedResult(started, "SSH_AUTH", "no ssh credential configured")
	}
	hostKeyCallback, err := hostKeyCallback(config, ec.KnownHostsPath)
	if err != nil {
		return failedResult(started, "SSH_HOST_KEY", err.Error())
	}
	clientConfig := &ssh.ClientConfig{
		User: username, Auth: auth, HostKeyCallback: hostKeyCallback,
		Timeout: time.Duration(ec.TimeoutSeconds) * time.Second,
	}
	address := net.JoinHostPort(host, fmt.Sprint(port))
	conn, err := net.DialTimeout("tcp", address, time.Duration(ec.TimeoutSeconds)*time.Second)
	if err != nil {
		return failedResult(started, "CONNECTION", err.Error())
	}
	clientConn, channels, requests, err := ssh.NewClientConn(conn, address, clientConfig)
	if err != nil {
		_ = conn.Close()
		return classifySSHConnectError(started, err)
	}
	client := ssh.NewClient(clientConn, channels, requests)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return failedResult(started, "CONNECTION", err.Error())
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()
	timedOut := false
	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(time.Duration(ec.TimeoutSeconds) * time.Second):
		timedOut = true
		_ = session.Close()
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	duration := int(time.Since(started).Milliseconds())
	outText, outCut := truncateBytes(stdout.Bytes(), ec.OutputLimitBytes)
	errText, errCut := truncateBytes(stderr.Bytes(), ec.OutputLimitBytes)
	if timedOut {
		class := "TIMEOUT"
		message := "ssh command timed out"
		return Result{Status: "timed_out", ErrorClass: &class, ErrorMessage: &message,
			Stdout: &outText, Stderr: &errText, DurationMS: duration, Truncated: outCut || errCut}
	}
	exitCode := 0
	if runErr != nil {
		var exitErr *ssh.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitStatus()
		} else {
			return classifySSHConnectError(started, runErr)
		}
	}
	class := "EXIT_CODE"
	message := fmt.Sprintf("ssh command exited with %d", exitCode)
	if exitCode == 0 {
		return Result{Status: "succeeded", ExitCode: &exitCode, Stdout: &outText,
			Stderr: &errText, DurationMS: duration, Truncated: outCut || errCut}
	}
	return Result{Status: "failed", ErrorClass: &class, ErrorMessage: &message,
		ExitCode: &exitCode, Stdout: &outText, Stderr: &errText,
		DurationMS: duration, Truncated: outCut || errCut}
}

func classifySSHConnectError(started time.Time, err error) Result {
	var keyErr *knownhosts.KeyError
	if errors.As(err, &keyErr) {
		return failedResult(started, "SSH_HOST_KEY", err.Error())
	}
	var authErr *ssh.ServerAuthError
	if errors.As(err, &authErr) {
		return failedResult(started, "SSH_AUTH", err.Error())
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return failedResult(started, "TIMEOUT", "ssh command timed out")
	}
	if strings.Contains(strings.ToLower(err.Error()), "unable to authenticate") {
		return failedResult(started, "SSH_AUTH", err.Error())
	}
	return failedResult(started, "CONNECTION", err.Error())
}

func hostKeyCallback(config map[string]any, globalPath string) (ssh.HostKeyCallback, error) {
	files := []string{}
	if value, _ := config["known_hosts"].(string); value != "" {
		files = append(files, value)
	} else if globalPath != "" {
		files = append(files, globalPath)
	} else {
		if home, err := os.UserHomeDir(); err == nil {
			files = append(files, filepath.Join(home, ".ssh", "known_hosts"))
		}
		files = append(files, "/etc/ssh/ssh_known_hosts")
	}
	existing := files[:0]
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			existing = append(existing, file)
		}
	}
	if len(existing) == 0 {
		return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return fmt.Errorf("no known_hosts file configured")
		}, nil
	}
	return knownhosts.New(existing...)
}
