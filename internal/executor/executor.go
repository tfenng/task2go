package executor

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/shlex"
)

type Context struct {
	RunID                   string
	AttemptNo               int
	Parameters              map[string]any
	Target                  map[string]any
	TimeoutSeconds          int
	OutputLimitBytes        int
	OutputCaptureLimitBytes int
	OutputRoot              string
	DefaultShell            string
	KnownHostsPath          string
	Credential              map[string]any
	SecretValues            []string
}

type Result struct {
	Status       string
	ErrorClass   *string
	ErrorMessage *string
	ExitCode     *int
	HTTPStatus   *int
	Stdout       *string
	Stderr       *string
	Response     *string
	DurationMS   int
	Truncated    bool
}

type Executor interface {
	Execute(context.Context, Context) Result
}

func Create(executorType string) (Executor, error) {
	switch executorType {
	case "local":
		return Local{}, nil
	case "ssh":
		return SSH{}, nil
	case "http":
		return HTTP{}, nil
	default:
		return nil, fmt.Errorf("unsupported executor type: %s", executorType)
	}
}

var placeholder = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

func render(value string, parameters map[string]any) (string, error) {
	var failure error
	result := placeholder.ReplaceAllStringFunc(value, func(match string) string {
		name := match[2 : len(match)-1]
		item, ok := parameters[name]
		if !ok {
			failure = fmt.Errorf("unknown parameter in argv: %s", name)
			return match
		}
		return fmt.Sprint(item)
	})
	return result, failure
}

func redact(text string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "***")
		}
	}
	return text
}

func truncateBytes(data []byte, limit int) (string, bool) {
	if len(data) <= limit {
		return strings.ToValidUTF8(string(data), "\uFFFD"), false
	}
	return strings.ToValidUTF8(string(data[:limit]), "\uFFFD"), true
}

func stringList(value any) []string {
	list, _ := value.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func mapValue(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func ShellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if arg == "" {
			quoted[i] = "''"
			continue
		}
		if strings.IndexFunc(arg, func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
				!(r >= '0' && r <= '9') && !strings.ContainsRune("_@%+=:,./-", r)
		}) == -1 {
			quoted[i] = arg
		} else {
			quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
		}
	}
	return strings.Join(quoted, " ")
}

func splitShebang(line string) []string {
	parts, err := shlex.Split(line)
	if err != nil {
		return nil
	}
	return parts
}
