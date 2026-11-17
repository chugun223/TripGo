package config

import (
	"fmt"
	"net/url"
	"time"
)

type PositionsConfig struct {
	CheckInterval time.Duration
	StaleAfter    time.Duration
	Workers       int
	QueueSize     int
	BatchSize     int
}

type PushConfig struct {
	HTTPURL        string
	RequestTimeout time.Duration
}

func loadPositions() (PositionsConfig, PushConfig, error) {
	var positions PositionsConfig
	var push PushConfig
	var err error

	positions.CheckInterval, err = positiveDuration("POSITION_CHECK_INTERVAL")
	if err != nil {
		return positions, push, err
	}
	positions.StaleAfter, err = positiveDuration("POSITION_STALE_AFTER")
	if err != nil {
		return positions, push, err
	}
	workers, err := positiveInt("POSITION_WORKERS")
	if err != nil {
		return positions, push, err
	}
	positions.Workers = workers
	queueSize, err := positiveInt("POSITION_QUEUE_SIZE")
	if err != nil {
		return positions, push, err
	}
	positions.QueueSize = queueSize
	batchSize, err := positiveInt("POSITION_SCAN_BATCH_SIZE")
	if err != nil {
		return positions, push, err
	}
	positions.BatchSize = batchSize
	push.RequestTimeout, err = positiveDuration("PUSH_REQUEST_TIMEOUT")
	if err != nil {
		return positions, push, err
	}
	push.HTTPURL, err = getRequiredEnv("PUSH_HTTP_URL")
	if err != nil {
		return positions, push, err
	}
	parsed, err := url.Parse(push.HTTPURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return positions, push, fmt.Errorf("PUSH_HTTP_URL must be an absolute HTTP or HTTPS URL")
	}

	return positions, push, nil
}

func positiveDuration(name string) (time.Duration, error) {
	value, err := getRequiredDuration(name)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return value, nil
}

func positiveInt(name string) (int, error) {
	value, err := getRequiredInt32(name)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return int(value), nil
}
