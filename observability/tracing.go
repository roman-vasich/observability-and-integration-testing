package observability

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Interactor[Req, Res any] interface {
	Execute(ctx context.Context, req Req) (Res, error)
}

type EventHandler[Event any] interface {
	Handle(ctx context.Context, event Event) error
}

type TracedInteractor[Req, Res any] struct {
	Inner  Interactor[Req, Res]
	Tracer trace.Tracer
	Name   string
}

type TracedEventHandler[Event any] struct {
	Inner  EventHandler[Event]
	Tracer trace.Tracer
	Name   string
}

func StartSpan(ctx context.Context, tracer trace.Tracer, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return tracer.Start(ctx, name, opts...)
}

func (t TracedInteractor[Req, Res]) Execute(ctx context.Context, req Req) (_ Res, err error) {
	tracer := defaultTracer(t.Tracer)
	name := defaultSpanName(t.Name, "usecase.execute")

	ctx, span := tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindInternal))
	defer finishSpan(span, &err)

	return t.Inner.Execute(ctx, req)
}

func (t TracedEventHandler[Event]) Handle(ctx context.Context, event Event) (err error) {
	tracer := defaultTracer(t.Tracer)
	name := defaultSpanName(t.Name, "domain.event")

	ctx, span := tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindConsumer))
	defer finishSpan(span, &err)

	return t.Inner.Handle(ctx, event)
}

func TraceDomainOperation[T any](ctx context.Context, tracer trace.Tracer, name string, fn func() (T, error)) (_ T, err error) {
	ctx, span := StartSpan(ctx, defaultTracer(tracer), defaultSpanName(name, "domain.operation"), trace.WithSpanKind(trace.SpanKindInternal))
	_ = ctx
	defer finishSpan(span, &err)

	return fn()
}

func TraceHttpMiddleware(tp trace.TracerProvider, name string) func(http.Handler) http.Handler {
	tracerProvider := tp
	if tracerProvider == nil {
		tracerProvider = otel.GetTracerProvider()
	}

	tracer := tracerProvider.Tracer(defaultSpanName(name, serviceName))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := tracer.Start(ctx, r.Method+" "+r.URL.Path, trace.WithSpanKind(trace.SpanKindServer))
			defer span.End()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func finishSpan(span trace.Span, err *error) {
	if rec := recover(); rec != nil {
		panicErr := panicError(rec)
		span.RecordError(panicErr)
		span.SetStatus(codes.Error, panicErr.Error())
		span.End()
		panic(rec)
	}

	if err != nil && *err != nil {
		span.RecordError(*err)
		span.SetStatus(codes.Error, (*err).Error())
	}

	span.End()
}

func defaultTracer(tracer trace.Tracer) trace.Tracer {
	if tracer != nil {
		return tracer
	}

	return otel.Tracer(serviceName)
}

func defaultSpanName(name, fallback string) string {
	if name != "" {
		return name
	}

	return fallback
}

func panicError(rec any) error {
	if err, ok := rec.(error); ok {
		return err
	}

	return fmt.Errorf("panic: %v", rec)
}
