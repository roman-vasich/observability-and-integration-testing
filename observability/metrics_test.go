package observability

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
)

func TestOrderMetrics_RecordsExemplarWithTraceID(t *testing.T) {
	metrics, err := NewOrderMetrics()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, metrics.Shutdown(context.Background()))
	})

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{16, 15, 14, 13, 12, 11, 10, 9},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	metrics.RecordCheckout(ctx, "premium", "api", "success", 850*time.Millisecond)

	rm, err := metrics.Collect(context.Background())
	require.NoError(t, err)

	hist := findHistogramMetric(t, rm, "order_checkout_duration_seconds")
	require.Len(t, hist.DataPoints, 1)
	require.NotEmpty(t, hist.DataPoints[0].Exemplars)

	ex := hist.DataPoints[0].Exemplars[0]
	require.Equal(t, sc.TraceID(), trace.TraceID(ex.TraceID))
	require.Equal(t, sc.SpanID(), trace.SpanID(ex.SpanID))
	require.InDelta(t, 0.85, ex.Value, 0.0001)
}

func TestOrderMetrics_PendingGaugeReflectsCurrentValue(t *testing.T) {
	metrics, err := NewOrderMetrics()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, metrics.Shutdown(context.Background()))
	})

	metrics.SetPending(7)

	rm, err := metrics.Collect(context.Background())
	require.NoError(t, err)

	gauge := findGaugeMetric(t, rm, "orders_pending_count")
	require.Len(t, gauge.DataPoints, 1)
	require.Equal(t, int64(7), gauge.DataPoints[0].Value)
}

func TestOrderMetrics_CollectWithExternalMeterDoesNotPanic(t *testing.T) {
	provider := sdkmetric.NewMeterProvider()
	metrics, err := NewOrderMetricsWithMeter(provider.Meter("test"))
	require.NoError(t, err)

	rm, err := metrics.Collect(context.Background())
	require.NoError(t, err)
	require.Empty(t, rm.ScopeMetrics)
}

func findHistogramMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Histogram[float64] {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			hist, ok := m.Data.(metricdata.Histogram[float64])
			require.True(t, ok, "metric %q is not a histogram", name)
			return hist
		}
	}

	t.Fatalf("metric %q not found", name)
	return metricdata.Histogram[float64]{}
}

func findGaugeMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Gauge[int64] {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			gauge, ok := m.Data.(metricdata.Gauge[int64])
			require.True(t, ok, "metric %q is not a gauge", name)
			return gauge
		}
	}

	t.Fatalf("metric %q not found", name)
	return metricdata.Gauge[int64]{}
}
