package main

import (
	"context"
	"fmt"
	"github.com/go-logr/stdr"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

// OtelService holds the tracer provider and exporter.
type OtelService struct {
	otelTraceProvider *sdktrace.TracerProvider
	otelExporter      sdktrace.SpanExporter
}

// NewOtelService creates a new OtelService.
func NewOtelService() OtelService {
	return OtelService{}
}

// Init sets up the OpenTelemetry service.
// It mimics the provided code by reading configuration,
// initializing the exporter, sampler, resource, propagators,
// and setting the global tracer provider.
func (s *OtelService) Init(ctx context.Context, zapLog *zap.Logger, config Config) error {
	// Use our custom autoexporter, which will handle the debug flags
	// Environment variables should have already been set in main()
	fmt.Println("OTLP Endpoint:", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	fmt.Println("Debug Mode:", os.Getenv("OTEL_DEBUG_EXPORTER"))

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return fmt.Errorf("initializing OTel exporter: %w", err)
	}

	serviceName := "demo-service"
	fmt.Println("Service Name:", serviceName)
	sampler, err := initSampler(serviceName, config)
	if err != nil {
		return fmt.Errorf("initializing OTel sampler: %w", err)
	}

	res, err := initResource(ctx, serviceName, config.Version, config.OtelConfig.Resource.Attributes)
	if err != nil {
		return fmt.Errorf("initializing OTel resources: %w", err)
	}

	s.otelExporter = exporter
	s.otelTraceProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithExportTimeout(time.Second*30)),
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(s.otelTraceProvider)
	// Set logging level to info to see SDK status messages
	stdr.SetVerbosity(1)

	// Log OTel errors to zap.
	otel.SetErrorHandler(otelErrHandler(func(err error) {
		fmt.Println("OTEL ERROR:", err.Error())
		zapLog.Error("OTel error", zap.Error(err))
	}))

	propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	otel.SetTextMapPropagator(propagator)

	fmt.Println("OpenTelemetry initialization complete with debug exporter")
	return nil
}

// Stop flushes the exporter and shuts down the tracer provider.
func (s *OtelService) Stop(ctx context.Context) error {
	if s.otelExporter != nil {
		if err := s.otelExporter.Shutdown(ctx); err != nil {
			return err
		}
	}

	if s.otelTraceProvider != nil {
		if err := s.otelTraceProvider.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

// otelErrHandler adapts a function to the otel.ErrorHandler interface.
type otelErrHandler func(err error)

func (o otelErrHandler) Handle(err error) {
	o(err)
}

var _ otel.ErrorHandler = otelErrHandler(nil)

// initResource creates an OTEL resource with attributes.
func initResource(ctx context.Context, serviceName, version string,
	attribs map[string]string) (*resource.Resource, error) {
	attrs := make([]attribute.KeyValue, 0, len(attribs))
	for k, v := range attribs {
		attrs = append(attrs, attribute.String(k, v))
	}

	return resource.New(ctx,
		resource.WithAttributes(attrs...),
		resource.WithProcessPID(),
		resource.WithProcessExecutableName(),
		resource.WithProcessExecutablePath(),
		resource.WithProcessOwner(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithProcessRuntimeDescription(),
		resource.WithHost(),
		resource.WithTelemetrySDK(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(version),
		),
	)
}

// initSampler determines which sampling strategy to use.
func initSampler(serviceName string, conf Config) (sdktrace.Sampler, error) {
	return sdktrace.AlwaysSample(), nil
}

// Config is a minimal configuration structure.
type Config struct {
	OpenTelemetryOtlpEndpoint string
	Version                   string
	OtelConfig                OtelConfig
}

type OtelConfig struct {
	Exporter ExporterConfig
	SDK      SDKConfig
	Service  ServiceConfig
	Traces   TracesConfig
	Resource ResourceConfig
}

type ExporterConfig struct {
	Otlp OtlpConfig
}

type OtlpConfig struct {
	Endpoint string
}

type SDKConfig struct {
	Disabled bool
}

type ServiceConfig struct {
	Name string
}

type TracesConfig struct {
	Sampler string
}

type ResourceConfig struct {
	Attributes map[string]string
}

type ManagerConfig struct {
	Host HostConfig
}

type HostConfig struct {
	Port string
}

type RefreshConfig struct {
	Interval string
}

type MaxConfig struct {
	Operations int
}

// --- Main CLI entry point ---

func main() {
	ctx := context.Background()
	zapLog, _ := zap.NewProduction()
	defer zapLog.Sync()

	// Set environment variables to configure the debug exporter
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://refinery.services.frasers.io:443/")
	os.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	os.Setenv("OTEL_DEBUG_EXPORTER", "true")           // Enable our custom debug exporter
	os.Setenv("OTEL_DEBUG_EXPORTER_DUMP_BODY", "true") // Optional: enable body dumping

	// For this example, we create a dummy configuration.
	cfg := Config{
		OpenTelemetryOtlpEndpoint: "https://refinery.services.frasers.io:443/", // non-empty to trigger exporter initialization
		Version:                   "v1.0.0",
		OtelConfig: OtelConfig{
			Exporter: ExporterConfig{
				Otlp: OtlpConfig{
					Endpoint: "", // will be set from OpenTelemetryOtlpEndpoint if empty
				},
			},
			SDK: SDKConfig{
				Disabled: false,
			},
			Service: ServiceConfig{
				Name: "demo-service",
			},
			Traces: TracesConfig{
				Sampler: "", // empty to use AlwaysSample
			},
			Resource: ResourceConfig{
				Attributes: map[string]string{
					"environment": "development",
					"debug_mode":  "true",
				},
			},
		},
	}

	fmt.Println("Initializing OpenTelemetry with debug exporter...")
	otelService := NewOtelService()
	if err := otelService.Init(ctx, zapLog, cfg); err != nil {
		zapLog.Fatal("failed to initialize OpenTelemetry", zap.Error(err))
	}
	defer func() {
		// Flush any remaining spans before exiting.
		ctxShutdown, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := otelService.Stop(ctxShutdown); err != nil {
			zapLog.Error("failed to shutdown OpenTelemetry", zap.Error(err))
		}
	}()

	fmt.Println("Starting trace generation...")
	tracer := otel.Tracer("demo-cli")

	// Create fewer spans for debugging purposes
	for i := 0; i < 10; i++ {
		_, span := tracer.Start(ctx, fmt.Sprintf("debug-span-%d", i))
		// Add some attributes to make the spans more interesting
		span.SetAttributes(
			attribute.String("iteration", fmt.Sprintf("%d", i)),
			attribute.Bool("debug", true),
			attribute.Int("value", i*10),
		)
		// Simulate work.
		time.Sleep(100 * time.Millisecond)
		span.End()

		fmt.Printf("Created and ended span %d/10\n", i+1)
	}

	fmt.Println("Forcing flush of spans...")
	err := otelService.otelTraceProvider.ForceFlush(ctx)
	if err != nil {
		fmt.Println("Error flushing spans:", err)
	} else {
		fmt.Println("Spans flushed successfully.")
	}

	fmt.Println("Debug trace complete. Check the console output for detailed OTLP HTTP information.")
	os.Exit(0)
}
