package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTP            HTTPConfig
	Log             LogConfig
	ShutdownTimeout time.Duration
	Database        DatabaseConfig
}

type HTTPConfig struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type LogConfig struct {
	Level string
}

type DatabaseConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	ConnectTimeout  time.Duration
	QueryTimeout    time.Duration
}

func getRequiredEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("required environment variable %s is not set", name)
	}

	return value, nil
}

func getRequiredInt32(name string) (int32, error) {
	value, err := getRequiredEnv(name)
	if err != nil {
		return 0, err
	}

	result, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}

	return int32(result), nil
}

func getRequiredDuration(name string) (time.Duration, error) {
	value, err := getRequiredEnv(name)
	if err != nil {
		return 0, err
	}

	result, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}

	return result, nil
}

func Load() (Config, error) {
	httpAddr, err := getRequiredEnv("HTTP_ADDR")
	if err != nil {
		return Config{}, err
	}

	logLevel, err := getRequiredEnv("LOG_LEVEL")
	if err != nil {
		return Config{}, err
	}

	databaseURL, err := getRequiredEnv("DATABASE_URL")
	if err != nil {
		return Config{}, err
	}

	maxConns, err := getRequiredInt32("DATABASE_MAX_CONNS")
	if err != nil {
		return Config{}, err
	}

	minConns, err := getRequiredInt32("DATABASE_MIN_CONNS")
	if err != nil {
		return Config{}, err
	}

	maxConnLifetime, err := getRequiredDuration("DATABASE_MAX_CONN_LIFETIME")
	if err != nil {
		return Config{}, err
	}

	connectTimeout, err := getRequiredDuration("DATABASE_CONNECT_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	queryTimeout, err := getRequiredDuration("DATABASE_QUERY_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := getRequiredDuration("SHUTDOWN_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	readTimeout, err := getRequiredDuration("HTTP_READ_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	readHeaderTimeout, err := getRequiredDuration("HTTP_READ_HEADER_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	writeTimeout, err := getRequiredDuration("HTTP_WRITE_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	idleTimeout, err := getRequiredDuration("HTTP_IDLE_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	config := Config{
		HTTP: HTTPConfig{
			Addr:              httpAddr,
			ReadTimeout:       readTimeout,
			ReadHeaderTimeout: readHeaderTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
		Log: LogConfig{
			Level: logLevel,
		},
		ShutdownTimeout: shutdownTimeout,
		Database: DatabaseConfig{
			URL:             databaseURL,
			MaxConns:        maxConns,
			MinConns:        minConns,
			MaxConnLifetime: maxConnLifetime,
			ConnectTimeout:  connectTimeout,
			QueryTimeout:    queryTimeout,
		},
	}

	return config, nil
}
