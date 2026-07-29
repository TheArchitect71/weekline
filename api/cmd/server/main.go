package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"weekline/api/internal/httpapi"
	"weekline/api/internal/store"
	"weekline/api/migrations"
)

func run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	db, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	data := store.New(db)
	if envBool("WEEKLINE_SEED_DEMO", false) {
		password := envOrDefault("WEEKLINE_DEMO_PASSWORD", "WeeklineDemo!")
		if err = data.SeedDemo(ctx, password); err != nil {
			return fmt.Errorf("seed demo data: %w", err)
		}
	}
	_ = data.DeleteExpiredSessions(ctx)

	address := envOrDefault("WEEKLINE_ADDRESS", "127.0.0.1:8080")
	handler := httpapi.NewHandler(data, httpapi.Config{
		CookieSecure:     envBool("WEEKLINE_COOKIE_SECURE", false),
		SessionTTL:       8 * time.Hour,
		HostControlToken: os.Getenv("WEEKLINE_HOST_CONTROL_TOKEN"),
		HostLeaseTTL:     envDuration("WEEKLINE_HOST_LEASE_TTL", 45*time.Second),
		RequireHostLease: envBool("WEEKLINE_REQUIRE_MANAGER_LEASE", false),
		EnableAttendance: envBool("WEEKLINE_ENABLE_ATTENDANCE", false),
	})
	server := &http.Server{
		Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("weekline api listening on http://%s", address)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("serve API: %w", err)
		}
	}()

	if caddy, err := startCaddy(ctx); err != nil {
		return err
	} else if caddy != nil {
		go func() {
			if err := caddy.Wait(); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("caddy stopped: %w", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
	case err = <-errCh:
		cancel()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
		log.Printf("graceful shutdown failed: %v", shutdownErr)
	}
	return err
}

func openDatabase(ctx context.Context) (*pgxpool.Pool, error) {
	databaseURL := envOrDefault("WEEKLINE_DATABASE_URL", "postgres://localhost/weekline_dev?sslmode=disable")
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("configure database: %w", err)
	}
	if err = db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err = migrations.Apply(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply database migrations: %w", err)
	}
	return db, nil
}

func bootstrapManager(ctx context.Context) error {
	db, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	manager, created, err := store.New(db).BootstrapManager(ctx, store.BootstrapManagerInput{
		Email:       os.Getenv("WEEKLINE_BOOTSTRAP_MANAGER_EMAIL"),
		DisplayName: os.Getenv("WEEKLINE_BOOTSTRAP_MANAGER_NAME"),
		Password:    os.Getenv("WEEKLINE_BOOTSTRAP_MANAGER_PASSWORD"),
	})
	if err != nil {
		return fmt.Errorf("bootstrap manager: %w", err)
	}
	if created {
		log.Printf("created initial Weekline manager %s", manager.Email)
	} else {
		log.Print("a Weekline manager already exists; bootstrap made no changes")
	}
	return nil
}

func hasArgument(value string) bool {
	for _, argument := range os.Args[1:] {
		if argument == value {
			return true
		}
	}
	return false
}

func startCaddy(ctx context.Context) (*exec.Cmd, error) {
	executable := strings.TrimSpace(os.Getenv("WEEKLINE_CADDY_EXECUTABLE"))
	if executable == "" {
		return nil, nil
	}
	config := strings.TrimSpace(os.Getenv("WEEKLINE_CADDY_CONFIG"))
	if config == "" {
		return nil, errors.New("WEEKLINE_CADDY_CONFIG is required when Caddy supervision is enabled")
	}
	command := exec.CommandContext(ctx, executable, "run", "--config", config, "--adapter", "caddyfile")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start caddy: %w", err)
	}
	log.Printf("caddy started with %s", config)
	return command, nil
}

func loadRuntimeConfig() error {
	for index := 1; index < len(os.Args); index++ {
		if os.Args[index] != "--config" {
			continue
		}
		if index+1 >= len(os.Args) {
			return errors.New("--config requires a file path")
		}
		return loadEnvFile(os.Args[index+1])
	}
	return nil
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open configuration %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\uFEFF"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid configuration line: %q", line)
		}
		key = strings.TrimSpace(key)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read configuration %s: %w", path, err)
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
