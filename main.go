package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/chronosphereio/demo-techstrong/petmetrics"
	"github.com/chronosphereio/demo-techstrong/pettracing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/metric/instrument/syncint64"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
)

var tracer = otel.Tracer("pet_app")

var metricCreateErr = "Unable to create metric."

type petMetrics struct {
	feedSuccess syncint64.Counter
	feedFailed  syncint64.Counter

	sleepSuccess syncint64.Counter
	sleepFailed  syncint64.Counter

	playSuccess syncint64.Counter
	playFailed  syncint64.Counter
}

type wrapperPetHandler struct {
	pm petMetrics
}

func initializePetMetrics(c metric.Meter) (*petMetrics, error) {
	return &petMetrics{
		feedSuccess:  createCounter(c, "feed_success", "Pet fed successfully."),
		feedFailed:   createCounter(c, "feed_failed", "Failed to feed pet."),
		sleepSuccess: createCounter(c, "sleep_success", "Pet slept successfully."),
		sleepFailed:  createCounter(c, "sleep_failed", "Pet failed to sleep successfully."),
		playSuccess:  createCounter(c, "play_success", "Played with pet successfully."),
		playFailed:   createCounter(c, "play_failed", "Failed to play with pet."),
	}, nil
}

func createCounter(m metric.Meter, name, desc string) syncint64.Counter {
	c, err := m.SyncInt64().Counter(
		name,
		instrument.WithUnit("1"),
		instrument.WithDescription(desc),
	)
	if err != nil {
		log.Fatal(metricCreateErr)
		return nil
	}

	return c
}

func play(ctx context.Context, pm petMetrics, t time.Duration, forcedErr bool) context.Context {
	ctx, span := tracer.Start(ctx, "play")
	defer span.End()

	if forcedErr {
		pm.playFailed.Add(ctx, 1)
		return ctx
	}

	time.Sleep(t)
	span.SetAttributes(attribute.Int("play.duration", int(t)))

	pm.playSuccess.Add(ctx, 1)
	return ctx
}

func feed(ctx context.Context, pm petMetrics, t time.Duration, forcedErr bool) context.Context {
	ctx, span := tracer.Start(ctx, "feed")
	defer span.End()

	if forcedErr {
		pm.feedFailed.Add(ctx, 1)
		return ctx
	}

	time.Sleep(t)
	span.SetAttributes(attribute.Int("feed.duration", int(t)))

	pm.feedSuccess.Add(ctx, 1)

	return ctx
}

func sleep(ctx context.Context, pm petMetrics, t time.Duration, forcedErr bool) context.Context {
	ctx, span := tracer.Start(ctx, "sleep")
	defer span.End()

	if forcedErr {
		pm.sleepFailed.Add(ctx, 1)
		return ctx
	}

	time.Sleep(t)
	span.SetAttributes(attribute.Int("sleep.duration", int(t)))

	pm.sleepSuccess.Add(ctx, 1)
	return ctx
}

func (ws wrapperPetHandler) petHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "pet-app")
	defer span.End()

	ctx = feed(ctx, ws.pm, 2*time.Second, false)
	ctx = play(ctx, ws.pm, 3*time.Second, false)
	ctx = sleep(ctx, ws.pm, 5*time.Second, false)

	fmt.Fprintf(w, "Congratulations!")
}

func main() {
	c := global.MeterProvider().Meter("pet_app")
	ctx := context.Background()

	pm, err := initializePetMetrics(c)
	if err != nil {
		log.Fatal("unable to initialize pet metrics.")
	}

	ws := wrapperPetHandler{
		pm: *pm,
	}

	configureOpentelemetry(ctx)

	// Wrap your httpHandler function.
	petHandler := http.HandlerFunc(ws.petHandler)
	wrappedSleepHandler := otelhttp.NewHandler(petHandler, "pet")
	http.Handle("/pet", wrappedSleepHandler)

	log.Fatal(http.ListenAndServe(":3035", nil))

}

func configureOpentelemetry(ctx context.Context) {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		// the service name used to display traces in backends
		semconv.ServiceNameKey.String("pet-service"),
	)

	pettracing.ConfigureTracing(ctx, res, tracer)
	petmetrics.ConfigureMetrics(res)
}
