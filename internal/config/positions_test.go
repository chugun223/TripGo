package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	checkIntervalEnv = "POSITION_CHECK_INTERVAL"
	pushURLEnv       = "PUSH_HTTP_URL"
	workersEnv       = "POSITION_WORKERS"
	batchSizeEnv     = "POSITION_SCAN_BATCH_SIZE"
)

func positionEnvironment(t *testing.T) {
	t.Helper()
	for name, value := range map[string]string{
		checkIntervalEnv: "30s", "POSITION_STALE_AFTER": "60s", workersEnv: "4",
		"POSITION_QUEUE_SIZE": "100", batchSizeEnv: "100", pushURLEnv: "http://localhost:8090", "PUSH_REQUEST_TIMEOUT": "1s",
	} {
		t.Setenv(name, value)
	}
}

func TestPositionConfiguration(t *testing.T) {
	positionEnvironment(t)
	positions, push, err := loadPositions()
	require.NoError(t, err)
	require.Equal(t, PositionsConfig{CheckInterval: 30 * time.Second, StaleAfter: time.Minute, Workers: 4, QueueSize: 100, BatchSize: 100}, positions)
	require.Equal(t, time.Second, push.RequestTimeout)
	for _, tc := range []struct{ name, value string }{
		{checkIntervalEnv, "0s"},
		{checkIntervalEnv, "invalid"},
		{"POSITION_STALE_AFTER", "-1s"},
		{workersEnv, "0"},
		{workersEnv, "many"},
		{"POSITION_QUEUE_SIZE", "-1"},
		{batchSizeEnv, "0"},
		{batchSizeEnv, "invalid"},
		{"PUSH_REQUEST_TIMEOUT", ""},
		{pushURLEnv, "localhost:8090"},
		{pushURLEnv, "file:///tmp/push"},
	} {
		t.Run(tc.name+"="+tc.value, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			_, _, err := loadPositions()
			require.ErrorContains(t, err, tc.name)
		})
	}
}
