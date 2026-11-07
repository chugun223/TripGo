package telemetry

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
)

const (
	serviceName      = "trip-service"
	serviceNamespace = "tripgo"
	defaultVersion   = "dev"
	defaultEnv       = "local"
)

func NewResource(ctx context.Context) (*resource.Resource, error) {
	res, err := resource.New(
		ctx,
		resource.WithFromEnv(),
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("service.version", serviceVersion()),
			attribute.String("service.namespace", serviceNamespace),
			attribute.String(
				"deployment.environment",
				deploymentEnvironment(),
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	return res, nil
}

func serviceVersion() string {
	if version := os.Getenv("SERVICE_VERSION"); version != "" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version != "" &&
		info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	return defaultVersion
}

func deploymentEnvironment() string {
	if env := os.Getenv("DEPLOYMENT_ENVIRONMENT"); env != "" {
		return env
	}

	return defaultEnv
}
