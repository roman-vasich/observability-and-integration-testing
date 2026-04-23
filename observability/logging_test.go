package observability

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestSecretString_LogValueRedacts(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{AddSource: false}))

	logger.Info("test", "secret", SecretString("super-secret"))

	out := buf.String()
	require.Contains(t, out, "[REDACTED]")
	require.NotContains(t, out, "super-secret")
}

func TestLoggingInteractor_LogsStartAndDoneWithAllowlistedAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{AddSource: false}))

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	type request struct {
		Method   string
		URI      string
		Password SecretString
	}

	type response struct {
		StatusCode int
	}

	wrapped := LoggingInteractor[request, response]{
		Inner: interactorFunc[request, response](func(ctx context.Context, req request) (response, error) {
			return response{StatusCode: 201}, nil
		}),
		Logger: logger,
		Name:   "order.create",
		RequestAttrs: func(req request) []slog.Attr {
			return []slog.Attr{
				slog.String("method", req.Method),
				slog.String("uri", req.URI),
				slog.Any("password", req.Password),
			}
		},
		ResponseAttrs: func(res response) []slog.Attr {
			return []slog.Attr{
				slog.Int("status_code", res.StatusCode),
			}
		},
	}

	result, err := wrapped.Execute(ctx, request{
		Method:   "POST",
		URI:      "/orders",
		Password: SecretString("super-secret"),
	})
	require.NoError(t, err)
	require.Equal(t, 201, result.StatusCode)

	out := buf.String()
	require.Contains(t, out, "order.create.start")
	require.Contains(t, out, "order.create.done")
	require.Contains(t, out, "trace_id=0102030405060708090a0b0c0d0e0f10")
	require.Contains(t, out, "method=POST")
	require.Contains(t, out, "uri=/orders")
	require.Contains(t, out, "status_code=201")
	require.Contains(t, out, "[REDACTED]")
	require.NotContains(t, out, "super-secret")
}

func TestLoggingInteractor_LogsError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{AddSource: false}))

	expected := errors.New("payment failed")
	wrapped := LoggingInteractor[string, string]{
		Inner: interactorFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return "", expected
		}),
		Logger: logger,
		Name:   "payment.authorize",
	}

	_, err := wrapped.Execute(context.Background(), "req")
	require.ErrorIs(t, err, expected)

	out := buf.String()
	require.Contains(t, out, "payment.authorize.start")
	require.Contains(t, out, "payment.authorize.error")
	require.Contains(t, out, "error=\"payment failed\"")
	require.Contains(t, out, "stack=")
}

func TestLoggingInteractor_LogsPanicWithStack(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{AddSource: false}))

	wrapped := LoggingInteractor[string, string]{
		Inner: interactorFunc[string, string](func(context.Context, string) (string, error) {
			panic("boom")
		}),
		Logger: logger,
		Name:   "payment.authorize",
	}

	require.PanicsWithValue(t, "boom", func() {
		_, _ = wrapped.Execute(context.Background(), "req")
	})

	out := buf.String()
	require.Contains(t, out, "payment.authorize.panic")
	require.Contains(t, out, "error=\"panic: boom\"")
	require.Contains(t, out, "stack=")
}

func TestLoggingInteractor_UsesDefaultLoggerWhenNil(t *testing.T) {
	var buf bytes.Buffer
	originalDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{AddSource: false})))
	t.Cleanup(func() {
		slog.SetDefault(originalDefault)
	})

	wrapped := LoggingInteractor[string, string]{
		Inner: interactorFunc[string, string](func(ctx context.Context, req string) (string, error) {
			return "ok", nil
		}),
		Name: "health.check",
	}

	_, err := wrapped.Execute(context.Background(), "req")
	require.NoError(t, err)
	require.True(t, strings.Contains(buf.String(), "health.check.done"))
}
