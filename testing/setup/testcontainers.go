package setup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	_ "github.com/lib/pq"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	DefaultPostgresImage  = "postgres:16-alpine"
	DefaultWireMockImage  = "wiremock/wiremock:2.35.0"
	defaultStartupTimeout = 60 * time.Second
)

type PostgresContainer struct {
	Container tc.Container
	DB        *sql.DB
	DSN       string
}

func StartPostgres(ctx context.Context, image string) (*PostgresContainer, error) {
	if image == "" {
		image = DefaultPostgresImage
	}

	ctr, err := tc.Run(ctx, image,
		tc.WithExposedPorts("5432/tcp"),
		tc.WithEnv(map[string]string{
			"POSTGRES_DB":       "orders",
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
		}),
		tc.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(defaultStartupTimeout)),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		_ = ctr.Terminate(ctx)
		return nil, fmt.Errorf("get postgres host: %w", err)
	}

	port, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		_ = ctr.Terminate(ctx)
		return nil, fmt.Errorf("get postgres port: %w", err)
	}

	dsn := fmt.Sprintf("postgres://test:test@%s/orders?sslmode=disable", net.JoinHostPort(host, port.Port()))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		_ = ctr.Terminate(ctx)
		return nil, fmt.Errorf("open postgres db: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pingWithRetry(pingCtx, db); err != nil {
		_ = db.Close()
		_ = ctr.Terminate(ctx)
		return nil, fmt.Errorf("ping postgres db: %w", err)
	}

	return &PostgresContainer{
		Container: ctr,
		DB:        db,
		DSN:       dsn,
	}, nil
}

func (p *PostgresContainer) Close(ctx context.Context) error {
	var errs []error
	if p.DB != nil {
		if err := p.DB.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if p.Container != nil {
		if err := p.Container.Terminate(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type WireMockContainer struct {
	Container tc.Container
	BaseURL   string
}

func StartWireMock(ctx context.Context, image string) (*WireMockContainer, error) {
	if image == "" {
		image = DefaultWireMockImage
	}

	ctr, err := tc.Run(ctx, image,
		tc.WithExposedPorts("8080/tcp"),
		tc.WithWaitStrategy(
			wait.ForHTTP("/__admin/mappings").
				WithPort("8080/tcp").
				WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
				WithForcedIPv4LocalHost().
				WithStartupTimeout(defaultStartupTimeout),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start wiremock container: %w", err)
	}

	baseURL, err := ctr.PortEndpoint(ctx, "8080/tcp", "http")
	if err != nil {
		_ = ctr.Terminate(ctx)
		return nil, fmt.Errorf("get wiremock endpoint: %w", err)
	}

	return &WireMockContainer{
		Container: ctr,
		BaseURL:   baseURL,
	}, nil
}

func (w *WireMockContainer) Close(ctx context.Context) error {
	if w.Container == nil {
		return nil
	}
	return w.Container.Terminate(ctx)
}

func pingWithRetry(ctx context.Context, db *sql.DB) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		if err := db.PingContext(ctx); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
