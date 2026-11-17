package push

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/chugun223/TripGo/internal/generated/pushapi"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestRequestPosition(t *testing.T) {
	for _, status := range []int{202, 400, 429, 503, 302} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			gateway := New("http://push.invalid/", time.Second)
			t.Cleanup(gateway.Close)
			driverID, tripID := uuid.New(), uuid.New()
			body := &trackedBody{Reader: strings.NewReader(`{"message_id":"accepted"}`)}
			calls := 0
			gateway.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/api/v1/push", r.URL.Path)
				require.Equal(t, "application/json", r.Header.Get("Content-Type"))
				deadline, ok := r.Context().Deadline()
				require.True(t, ok, "client must bound the request")
				require.LessOrEqual(t, time.Until(deadline), time.Second)
				var payload pushapi.SendPushJSONRequestBody
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				require.Equal(t, driverID, payload.RecipientId)
				require.Equal(t, pushapi.REQUESTPOSITION, payload.Kind)
				_, err := uuid.Parse(payload.RequestId)
				require.NoError(t, err)
				require.NotNil(t, payload.Data)
				require.Equal(t, tripID.String(), (*payload.Data)["trip_id"])
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: body}, nil
			})
			err := gateway.RequestPosition(context.Background(), driverID, tripID)
			if status == http.StatusAccepted {
				require.NoError(t, err)
			} else {
				var statusErr *StatusError
				require.ErrorAs(t, err, &statusErr)
				require.Equal(t, status, statusErr.Code)
				require.Contains(t, statusErr.Error(), strconv.Itoa(status))
			}
			require.Equal(t, 1, calls, "lab 3 must not retry requests")
			require.True(t, body.closed, "response body must be closed even on 4xx/5xx")
		})
	}
}

func TestTransportFailureAndCancellation(t *testing.T) {
	gateway := New("http://push.invalid", time.Second)
	t.Cleanup(gateway.Close)
	transportErr := errors.New("connection refused")
	gateway.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, transportErr })
	err := gateway.RequestPosition(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, transportErr)
	var statusErr *StatusError
	require.False(t, errors.As(err, &statusErr))
	started := make(chan struct{})
	gateway.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- gateway.RequestPosition(ctx, uuid.New(), uuid.New()) }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}
