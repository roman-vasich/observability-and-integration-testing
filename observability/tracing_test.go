package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceHttpMiddleware_ExtractsParentAndStartsServerSpan(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))

	originalPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTextMapPropagator(originalPropagator)
		require.NoError(t, tp.Shutdown(context.Background()))
	})

	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		SpanID:     trace.SpanID{2, 2, 2, 2, 2, 2, 2, 2},
		TraceFlags: trace.FlagsSampled,
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/orders", nil)
	otel.GetTextMapPropagator().Inject(trace.ContextWithSpanContext(context.Background(), parent), propagation.HeaderCarrier(req.Header))

	var seen trace.SpanContext
	handler := TraceHttpMiddleware(tp, "orders-http")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = trace.SpanContextFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
	require.True(t, seen.IsValid())
	require.Equal(t, parent.TraceID(), seen.TraceID())
	require.NotEqual(t, parent.SpanID(), seen.SpanID())

	spans := sr.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "GET /orders", spans[0].Name())
	require.Equal(t, trace.SpanKindServer, spans[0].SpanKind())
	require.Equal(t, parent.TraceID(), spans[0].Parent().TraceID())
	require.Equal(t, parent.SpanID(), spans[0].Parent().SpanID())
}

func TestTracedInteractor_StartsSpanAndPropagatesContext(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	tracer := tp.Tracer("test")
	t.Cleanup(func() {
		require.NoError(t, tp.Shutdown(context.Background()))
	})

	inner := interactorFunc[string, string](func(ctx context.Context, req string) (string, error) {
		sc := trace.SpanContextFromContext(ctx)
		require.True(t, sc.IsValid())
		return req + "-done", nil
	})

	wrapped := TracedInteractor[string, string]{
		Inner:  inner,
		Tracer: tracer,
		Name:   "order.create",
	}

	result, err := wrapped.Execute(context.Background(), "req")
	require.NoError(t, err)
	require.Equal(t, "req-done", result)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "order.create", spans[0].Name())
	require.Equal(t, trace.SpanKindInternal, spans[0].SpanKind())
	require.Equal(t, codes.Unset, spans[0].Status().Code)
}

func TestTraceDomainOperation_RecordsErrorWithoutPassingContextToDomain(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	tracer := tp.Tracer("test")
	t.Cleanup(func() {
		require.NoError(t, tp.Shutdown(context.Background()))
	})

	expected := errors.New("domain failed")
	_, err := TraceDomainOperation(context.Background(), tracer, "order.complete", func() (string, error) {
		return "", expected
	})
	require.ErrorIs(t, err, expected)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "order.complete", spans[0].Name())
	require.Equal(t, codes.Error, spans[0].Status().Code)
	require.Equal(t, expected.Error(), spans[0].Status().Description)
}

func TestTracedEventHandler_StartsConsumerSpan(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	tracer := tp.Tracer("test")
	t.Cleanup(func() {
		require.NoError(t, tp.Shutdown(context.Background()))
	})

	handler := TracedEventHandler[string]{
		Inner: eventHandlerFunc[string](func(ctx context.Context, event string) error {
			require.Equal(t, "order_placed", event)
			require.True(t, trace.SpanContextFromContext(ctx).IsValid())
			return nil
		}),
		Tracer: tracer,
		Name:   "order.event",
	}

	err := handler.Handle(context.Background(), "order_placed")
	require.NoError(t, err)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "order.event", spans[0].Name())
	require.Equal(t, trace.SpanKindConsumer, spans[0].SpanKind())
}

type interactorFunc[Req, Res any] func(ctx context.Context, req Req) (Res, error)

func (f interactorFunc[Req, Res]) Execute(ctx context.Context, req Req) (Res, error) {
	return f(ctx, req)
}

type eventHandlerFunc[Event any] func(ctx context.Context, event Event) error

func (f eventHandlerFunc[Event]) Handle(ctx context.Context, event Event) error {
	return f(ctx, event)
}
