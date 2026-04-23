package observability

import (
	"context"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// OrderMetrics captures order-processing metrics with trace-linked exemplars.
type OrderMetrics struct {
	provider *sdkmetric.MeterProvider
	reader   *sdkmetric.ManualReader

	ordersCreated       otelmetric.Int64Counter
	checkoutDuration    otelmetric.Float64Histogram
	stepDuration        otelmetric.Float64Histogram
	dependencyDuration  otelmetric.Float64Histogram
	fulfillmentDelay    otelmetric.Float64Histogram
	pendingOrders       otelmetric.Int64ObservableGauge
	pending             atomic.Int64
	pendingRegistration otelmetric.Registration
}

// NewOrderMetrics creates an exemplar-enabled metrics runtime for order processing.
func NewOrderMetrics() (*OrderMetrics, error) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithExemplarFilter(exemplar.TraceBasedFilter),
	)

	metrics, err := newOrderMetricsFromMeter(provider.Meter(serviceName))
	if err != nil {
		return nil, err
	}

	metrics.provider = provider
	metrics.reader = reader
	return metrics, nil
}

// NewOrderMetricsWithMeter builds the instruments on an existing meter.
// NOTE: It does not manage a reader/provider lifecycle.
func NewOrderMetricsWithMeter(meter otelmetric.Meter) (*OrderMetrics, error) {
	return newOrderMetricsFromMeter(meter)
}

// Collect reads the current metric state from the internal reader and returns the collected metrics.
func (m *OrderMetrics) Collect(ctx context.Context) (metricdata.ResourceMetrics, error) {
	var rm metricdata.ResourceMetrics
	if m == nil || m.reader == nil {
		return rm, nil
	}
	return rm, m.reader.Collect(ctx, &rm)
}

func newOrderMetricsFromMeter(meter otelmetric.Meter) (*OrderMetrics, error) {
	counter, err := meter.Int64Counter(
		"orders_created_total",
		otelmetric.WithDescription("Number of orders created by outcome."),
	)
	if err != nil {
		return nil, err
	}

	pending, err := meter.Int64ObservableGauge(
		"orders_pending_count",
		otelmetric.WithDescription("Number of orders waiting to be fulfilled."),
	)
	if err != nil {
		return nil, err
	}

	checkout, err := meter.Float64Histogram(
		"order_checkout_duration_seconds",
		otelmetric.WithDescription("End-to-end order checkout duration."),
		otelmetric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	step, err := meter.Float64Histogram(
		"order_processing_duration_seconds",
		otelmetric.WithDescription("Duration of order processing steps."),
		otelmetric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	dependency, err := meter.Float64Histogram(
		"dependency_call_duration_seconds",
		otelmetric.WithDescription("Duration of dependency calls during order processing."),
		otelmetric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	fulfillment, err := meter.Float64Histogram(
		"order_fulfillment_delay_seconds",
		otelmetric.WithDescription("Delay from order creation to fulfillment completion."),
		otelmetric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	metrics := &OrderMetrics{
		ordersCreated:      counter,
		checkoutDuration:   checkout,
		stepDuration:       step,
		dependencyDuration: dependency,
		fulfillmentDelay:   fulfillment,
		pendingOrders:      pending,
	}

	reg, err := meter.RegisterCallback(func(ctx context.Context, obs otelmetric.Observer) error {
		obs.ObserveInt64(metrics.pendingOrders, metrics.pending.Load())
		return nil
	}, metrics.pendingOrders)
	if err != nil {
		return nil, err
	}
	metrics.pendingRegistration = reg

	return metrics, nil
}

func (m *OrderMetrics) RecordCheckout(ctx context.Context, customerTier, channel, status string, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("status", status),
		attribute.String("customer_tier", customerTier),
		attribute.String("channel", channel),
	}

	if m.ordersCreated != nil {
		m.ordersCreated.Add(ctx, 1, otelmetric.WithAttributes(attrs...))
	}
	if m.checkoutDuration != nil {
		m.checkoutDuration.Record(ctx, duration.Seconds(), otelmetric.WithAttributes(attrs...))
	}
}

func (m *OrderMetrics) RecordStep(ctx context.Context, step, outcome string, duration time.Duration) {
	m.stepDuration.Record(
		ctx,
		duration.Seconds(),
		otelmetric.WithAttributes(
			attribute.String("step", step),
			attribute.String("outcome", outcome),
		),
	)
}

func (m *OrderMetrics) RecordDependency(ctx context.Context, dependency, operation, outcome string, duration time.Duration) {
	m.dependencyDuration.Record(
		ctx,
		duration.Seconds(),
		otelmetric.WithAttributes(
			attribute.String("dependency", dependency),
			attribute.String("operation", operation),
			attribute.String("outcome", outcome),
		),
	)
}

func (m *OrderMetrics) RecordFulfillmentDelay(ctx context.Context, duration time.Duration, outcome string) {
	m.fulfillmentDelay.Record(
		ctx,
		duration.Seconds(),
		otelmetric.WithAttributes(
			attribute.String("outcome", outcome),
		),
	)
}

func (m *OrderMetrics) AddPending(delta int64) {
	m.pending.Add(delta)
}

func (m *OrderMetrics) SetPending(value int64) {
	m.pending.Store(value)
}

func (m *OrderMetrics) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}

	if m.pendingRegistration != nil {
		_ = m.pendingRegistration.Unregister()
	}
	if m.provider != nil {
		return m.provider.Shutdown(ctx)
	}
	return nil
}
