package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/chugun223/TripGo/internal/generated/pushapi"
)

type Gateway struct {
	baseURL   string
	client    *http.Client
	transport *http.Transport
}

type StatusError struct {
	Code int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("push service returned HTTP %d", e.Code)
}

func New(baseURL string, timeout time.Duration) *Gateway {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: timeout, KeepAlive: timeout}).DialContext,
		TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout, IdleConnTimeout: timeout,
		MaxIdleConns: 100, MaxIdleConnsPerHost: 16,
	}
	return &Gateway{
		baseURL: strings.TrimRight(baseURL, "/"), transport: transport,
		client: &http.Client{
			Timeout: timeout, Transport: otelhttp.NewTransport(transport),
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (g *Gateway) RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error {
	data := map[string]string{"trip_id": tripID.String()}
	body := pushapi.SendPushJSONRequestBody{
		RequestId: uuid.NewString(), RecipientId: driverID, Kind: pushapi.REQUESTPOSITION, Data: &data,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode position push request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/api/v1/push", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build position push request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(request)
	if err != nil {
		return fmt.Errorf("send position push request: %w", err)
	}
	defer response.Body.Close()

	_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode != http.StatusAccepted {
		return &StatusError{Code: response.StatusCode}
	}
	if readErr != nil {
		return fmt.Errorf("read position push response: %w", readErr)
	}
	return nil
}

func (g *Gateway) Close() { g.transport.CloseIdleConnections() }
