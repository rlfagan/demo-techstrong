package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/metric/instrument/syncint64"
	"go.opentelemetry.io/otel/sdk/metric/aggregator/histogram"
	otelcontroller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	"go.opentelemetry.io/otel/sdk/metric/export/aggregation"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	selector "go.opentelemetry.io/otel/sdk/metric/selector/simple"
)

// Package-level tracer.
// This should be configured in your code setup instead of here.
var tracer = otel.Tracer("github.com/full/path/to/mypkg")

type demoMetrics struct {
	successes syncint64.Counter
}

// sleepy mocks work that your application does.
func sleepy(ctx context.Context, dm demoMetrics) {
	_, span := tracer.Start(ctx, "sleep")
	defer span.End()

	sleepTime := 1 * time.Second
	time.Sleep(sleepTime)
	span.SetAttributes(attribute.Int("sleep.duration", int(sleepTime)))

	dm.successes.Add(ctx, 1)
}

// httpHandler is an HTTP handler function that is going to be instrumented.
func httpHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, World! I am instrumented automatically!")
	ctx := r.Context()

	c := global.MeterProvider().Meter("app_or_package_name")
	counter, err := c.SyncInt64().Counter(
		"test.my_counter",
		instrument.WithUnit("1"),
		instrument.WithDescription("Just a test counter"),
	)
	if err != nil {
		log.Fatal("Unable to create counter.")
	}

	demoMetrics := demoMetrics{
		successes: counter,
	}

	sleepy(ctx, demoMetrics)
}

func main() {

	// Wrap your httpHandler function.
	handler := http.HandlerFunc(httpHandler)
	wrappedHandler := otelhttp.NewHandler(handler, "hello-instrumented")
	http.Handle("/hello-instrumented", wrappedHandler)

	go func() {
		configureOpentelemetry()
	}()
	// And start the HTTP serve.
	log.Fatal(http.ListenAndServe(":3030", nil))

}

func configureOpentelemetry() {

	exporter := configureMetrics()

	http.HandleFunc("/metrics", exporter.ServeHTTP)
	fmt.Println("listenening on http://localhost:8088/metrics")

	go func() {
		_ = http.ListenAndServe(":8088", nil)
	}()
}

func configureMetrics() *prometheus.Exporter {
	config := prometheus.Config{}

	ctrl := otelcontroller.New(
		processor.NewFactory(
			selector.NewWithHistogramDistribution(
				histogram.WithExplicitBoundaries(config.DefaultHistogramBoundaries),
			),
			// export.CumulativeExportKindSelector(),
			aggregation.CumulativeTemporalitySelector(),
			processor.WithMemory(true),
		),
	)

	exporter, err := prometheus.New(config, ctrl)
	if err != nil {
		panic(err)
	}

	global.SetMeterProvider(exporter.MeterProvider())

	return exporter
}
