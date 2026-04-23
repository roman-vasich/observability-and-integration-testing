package observability

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type SecretString string

func (SecretString) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}

type LoggingInteractor[Req, Res any] struct {
	Inner         Interactor[Req, Res]
	Logger        *slog.Logger
	Name          string
	RequestAttrs  func(req Req) []slog.Attr
	ResponseAttrs func(res Res) []slog.Attr
}

func (l LoggingInteractor[Req, Res]) Execute(ctx context.Context, req Req) (_ Res, err error) {
	logger := appLogger(l.Logger)
	name := logName(l.Name, "usecase")
	start := time.Now()
	traceID := traceID(ctx)

	logger.LogAttrs(ctx, slog.LevelInfo, name+".start",
		append([]slog.Attr{
			slog.String("trace_id", traceID),
		}, logAttrs(l.RequestAttrs, req)...)...,
	)

	defer func() {
		if rec := recover(); rec != nil {
			panicErr := panicError(rec)
			logger.LogAttrs(ctx, slog.LevelError, name+".panic",
				slog.String("trace_id", traceID),
				slog.String("error", panicErr.Error()),
				slog.String("stack", string(debug.Stack())),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
			panic(rec)
		}

		if err != nil {
			logger.LogAttrs(ctx, slog.LevelError, name+".error",
				slog.String("trace_id", traceID),
				slog.String("error", err.Error()),
				slog.String("stack", string(debug.Stack())),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
			return
		}
	}()

	res, err := l.Inner.Execute(ctx, req)
	if err != nil {
		return res, err
	}

	logger.LogAttrs(ctx, slog.LevelInfo, name+".done",
		append([]slog.Attr{
			slog.String("trace_id", traceID),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		}, logAttrs(l.ResponseAttrs, res)...)...,
	)

	return res, nil
}

func appLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}

	return slog.Default()
}

func logName(name, fallback string) string {
	if name != "" {
		return name
	}

	return fallback
}

func logAttrs[T any](fn func(T) []slog.Attr, value T) []slog.Attr {
	if fn == nil {
		return nil
	}

	return fn(value)
}

func traceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}

	return sc.TraceID().String()
}
