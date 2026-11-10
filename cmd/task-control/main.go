package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"
	"github.com/tfenng/tasks/task2go/internal/api"
	"github.com/tfenng/tasks/task2go/internal/config"
	"github.com/tfenng/tasks/task2go/internal/executor"
	"github.com/tfenng/tasks/task2go/internal/logging"
	"github.com/tfenng/tasks/task2go/internal/scheduler"
	"github.com/tfenng/tasks/task2go/internal/security"
	"github.com/tfenng/tasks/task2go/internal/store"
	"github.com/tfenng/tasks/task2go/internal/web"
	"github.com/tfenng/tasks/task2go/internal/worker"
	"golang.org/x/term"
)

const version = "0.1.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "create-admin":
			os.Exit(runCreateAdmin(os.Args[2:]))
		case "reset-password":
			os.Exit(runResetPassword(os.Args[2:]))
		case "retention":
			os.Exit(runRetention(os.Args[2:]))
		case "migrate":
			os.Exit(runMigrate(os.Args[2:]))
		case "healthcheck":
			os.Exit(runHealthcheck(os.Args[2:]))
		case "version":
			fmt.Println(version)
			return
		case "serve":
			os.Exit(runServe(os.Args[2:]))
		}
	}
	os.Exit(runServe(os.Args[1:]))
}

func setup() (*config.Config, *store.Store, *security.SecretBox, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	logging.Configure(cfg.LogLevel)
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := st.Migrate(ctx); err != nil {
		st.Close()
		return nil, nil, nil, err
	}
	return cfg, st, security.NewSecretBox(cfg.SecretKeys, cfg.SecretActiveVersion), nil
}

func runServe(args []string) int {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	role := flags.String("role", "all", "all, api, worker or scheduler (comma separated)")
	_ = flags.Parse(args)
	cfg, st, box, err := setup()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer st.Close()
	roles := map[string]bool{}
	for _, item := range strings.Split(*role, ",") {
		roles[strings.TrimSpace(item)] = true
	}
	if roles["all"] || len(roles) == 0 {
		roles = map[string]bool{"api": true, "worker": true, "scheduler": true}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	errCh := make(chan error, 3)
	if roles["worker"] {
		go func() { errCh <- worker.New(cfg, st, box).Run(ctx) }()
	}
	if roles["scheduler"] {
		go func() { errCh <- scheduler.New(cfg, st, box).Run(ctx) }()
	}
	var server *http.Server
	if roles["api"] {
		distFS, _ := web.Dist()
		index, _ := readIndex(distFS)
		if len(index) == 0 {
			slog.Warn("frontend dist is empty; serve the UI after running the build script")
		}
		handler := api.NewServer(cfg, st, box, http.FS(distFS), index).Handler()
		server = &http.Server{
			Addr: cfg.HTTPAddr, Handler: handler,
			ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
		}
		go func() {
			slog.Info("http server listening", "addr", cfg.HTTPAddr)
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
	}
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			slog.Error("service stopped", "error", err)
			cancel()
			return 1
		}
	}
	if server != nil {
		shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = server.Shutdown(shutdownCtx)
	}
	return 0
}

func readIndex(distFS fs.FS) ([]byte, error) {
	if distFS == nil {
		return nil, nil
	}
	return fs.ReadFile(distFS, "index.html")
}

func runMigrate(args []string) int {
	flags := flag.NewFlagSet("migrate", flag.ExitOnError)
	_ = flags.Parse(args)
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	logging.Configure(cfg.LogLevel)
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("migrations applied")
	return 0
}

func runCreateAdmin(args []string) int {
	flags := flag.NewFlagSet("create-admin", flag.ExitOnError)
	username := flags.String("username", "", "username")
	displayName := flags.String("display-name", "", "display name")
	password := flags.String("password", "", "password (prompted when omitted)")
	_ = flags.Parse(args)
	if *username == "" {
		fmt.Fprintln(os.Stderr, "--username is required")
		return 1
	}
	_, st, _, err := setup()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer st.Close()
	ctx := context.Background()
	existing, err := st.GetUserByUsernameCI(ctx, st.Pool, *username)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if existing != nil {
		fmt.Fprintf(os.Stderr, "user already exists: %s\n", *username)
		return 1
	}
	plain, err := readPassword(*password)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	hash, err := security.HashPassword(plain)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *displayName == "" {
		*displayName = *username
	}
	now := time.Now().UTC()
	user := &store.User{
		ID: store.NewID(), Username: *username, DisplayName: *displayName,
		PasswordHash: hash, Role: "admin", IsActive: true, CreatedAt: now,
		UpdatedAt: now, Version: 1,
	}
	if err := st.CreateUser(ctx, st.Pool, user); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("created admin: %s\n", *username)
	return 0
}

func runResetPassword(args []string) int {
	flags := flag.NewFlagSet("reset-password", flag.ExitOnError)
	username := flags.String("username", "", "username")
	password := flags.String("password", "", "password (prompted when omitted)")
	_ = flags.Parse(args)
	if *username == "" {
		fmt.Fprintln(os.Stderr, "--username is required")
		return 1
	}
	_, st, _, err := setup()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer st.Close()
	ctx := context.Background()
	user, err := st.GetUserByUsernameCI(ctx, st.Pool, *username)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if user == nil {
		fmt.Fprintf(os.Stderr, "user not found: %s\n", *username)
		return 1
	}
	plain, err := readPassword(*password)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	hash, err := security.HashPassword(plain)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	user.PasswordHash = hash
	user.MustChangePassword = true
	user.Version++
	if err := st.SaveUser(ctx, st.Pool, user); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("password reset: %s\n", *username)
	return 0
}

func runRetention(args []string) int {
	flags := flag.NewFlagSet("retention", flag.ExitOnError)
	_ = flags.Parse(args)
	cfg, st, _, err := setup()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	result := map[string]int{}
	err = st.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = st.RetentionDelete(ctx, tx, now,
			now.AddDate(0, 0, -cfg.RetentionDays),
			now.AddDate(0, 0, -cfg.AttemptRetentionDays))
		return err
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	result["outputs"] = executor.CleanupOutputRoot(cfg.OutputRoot, time.Duration(cfg.AttemptRetentionDays)*24*time.Hour)
	fmt.Println(result)
	return 0
}

func runHealthcheck(args []string) int {
	flags := flag.NewFlagSet("healthcheck", flag.ExitOnError)
	url := flags.String("url", "", "health URL")
	_ = flags.Parse(args)
	if *url == "" {
		addr := os.Getenv("TASK_CONTROL_HTTP_ADDR")
		if addr == "" {
			addr = "127.0.0.1:8000"
		}
		*url = strings.Replace(addr, "0.0.0.0", "127.0.0.1", 1)
		*url = "http://" + *url + "/api/v1/health/live"
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(*url)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %s\n", resp.Status)
		return 1
	}
	return 0
}

func readPassword(value string) (string, error) {
	if value != "" {
		if len(value) < 6 {
			return "", errors.New("password must be at least 6 characters")
		}
		return value, nil
	}
	fmt.Fprint(os.Stderr, "Password: ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Confirm password: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", errors.New("passwords do not match")
	}
	if len(first) < 6 {
		return "", errors.New("password must be at least 6 characters")
	}
	return string(first), nil
}
