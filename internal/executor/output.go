package executor

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	StdoutName = "stdout.log"
	StderrName = "stderr.log"
)

func AttemptDir(root, runID string, attemptNo int) string {
	return filepath.Join(root, runID, itoa(attemptNo))
}

func EnsureAttemptDir(root, runID string, attemptNo int) (string, error) {
	dir := AttemptDir(root, runID, attemptNo)
	return dir, os.MkdirAll(dir, 0o755)
}

func ReadTail(path string, maxBytes int) (string, int, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", 0, false
	}
	total := int(info.Size())
	if maxBytes <= 0 {
		return "", total, total > 0
	}
	if total > maxBytes {
		if _, err := file.Seek(int64(total-maxBytes), io.SeekStart); err != nil {
			return "", total, false
		}
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return "", total, false
	}
	return strings.ToValidUTF8(string(data), "\uFFFD"), total, total > maxBytes
}

func TailText(text string, maxBytes int) (string, int, bool) {
	data := []byte(text)
	if maxBytes <= 0 || len(data) <= maxBytes {
		return text, len(data), false
	}
	return strings.ToValidUTF8(string(data[len(data)-maxBytes:]), "\uFFFD"), len(data), true
}

// Pump copies a process stream to a file, redacting secret values and capping
// the file at limit bytes. It flushes after every chunk so the API can tail it.
func Pump(reader io.Reader, path string, limit int, secrets []string) (int, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, false, err
	}
	defer file.Close()
	if limit < 1 {
		limit = 1
	}
	buf := make([]byte, 64*1024)
	written := 0
	truncated := false
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			text := strings.ToValidUTF8(string(buf[:n]), "\uFFFD")
			for _, secret := range secrets {
				if secret != "" {
					text = strings.ReplaceAll(text, secret, "***")
				}
			}
			data := []byte(text)
			if written >= limit {
				truncated = true
			} else {
				remaining := limit - written
				if len(data) > remaining {
					data = data[:remaining]
					truncated = true
				}
				if len(data) > 0 {
					if _, err := file.Write(data); err != nil {
						return written, truncated, err
					}
					written += len(data)
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return written, truncated, readErr
		}
	}
	return written, truncated, nil
}

func CleanupRunOutput(root, runID string) {
	_ = os.RemoveAll(filepath.Join(root, runID))
}

func CleanupOutputRoot(root string, cutoff time.Duration) int {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	removed := 0
	now := time.Now()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) >= cutoff {
			if os.RemoveAll(filepath.Join(root, entry.Name())) == nil {
				removed++
			}
		}
	}
	return removed
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
