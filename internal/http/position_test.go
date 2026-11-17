package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	api "github.com/chugun223/TripGo/internal/generated"
)

func TestPositionPathValidation(t *testing.T) {
	handler := api.HandlerWithOptions(&Handler{}, api.ChiServerOptions{ErrorHandlerFunc: PositionParameterError})
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, "/api/v1/trips/not-a-uuid/positions", nil))
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
		require.Contains(t, w.Body.String(), `"code":"invalid_request"`)
	}
}
