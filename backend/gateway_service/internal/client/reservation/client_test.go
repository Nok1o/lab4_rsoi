package reservation

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestClientOperations(t *testing.T) {
	reservationUID, bookUID, libraryUID := uuid.New(), uuid.New(), uuid.New()
	responseBody := `{"reservationUid":"` + reservationUID.String() + `","bookUid":"` + bookUID.String() + `","libraryUid":"` + libraryUID.String() + `","status":"RENTED","startDate":"2026-09-20","tillDate":"2026-09-22"}`
	returnBody := strings.Replace(responseBody, `"status":"RENTED"`, `"status":"RETURNED"`, 1)
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("X-User-Name") != "alice" {
			t.Errorf("X-User-Name = %q", request.Header.Get("X-User-Name"))
		}
		var code int
		var body string
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/reservations" && request.URL.RawQuery == "":
			code, body = http.StatusOK, "["+responseBody+"]"
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/reservations/count" && request.URL.Query().Get("status") == "RENTED":
			code, body = http.StatusOK, `{"count":1}`
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/reservations/"+reservationUID.String():
			code, body = http.StatusOK, responseBody
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/reservations":
			encoded, err := io.ReadAll(request.Body)
			if err != nil || !strings.Contains(string(encoded), `"tillDate":"2026-09-22"`) {
				t.Errorf("rent request = %s, %v", encoded, err)
			}
			code, body = http.StatusCreated, responseBody
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/reservations/"+reservationUID.String()+"/return":
			encoded, err := io.ReadAll(request.Body)
			if err != nil || !strings.Contains(string(encoded), `"date":"2026-09-22"`) {
				t.Errorf("return request = %s, %v", encoded, err)
			}
			code, body = http.StatusOK, returnBody
		case request.Method == http.MethodDelete && request.URL.Path == "/api/v1/reservations/"+reservationUID.String():
			code = http.StatusNoContent
		default:
			t.Errorf("unexpected request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
			code, body = http.StatusNotFound, `{"message":"not found"}`
		}
		return &http.Response{
			StatusCode: code,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	client, err := NewClient("http://reservation-service:8070", &http.Client{Transport: transport}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	items, err := client.ListByUsername(ctx, "alice")
	if err != nil || len(items) != 1 || items[0].Id != reservationUID || items[0].Username != "alice" {
		t.Fatalf("ListByUsername = (%+v, %v)", items, err)
	}
	count, err := client.CountByUsernameAndStatus(ctx, "alice", domain.StatusRented)
	if err != nil || count != 1 {
		t.Fatalf("CountByUsernameAndStatus = (%d, %v)", count, err)
	}
	got, err := client.GetByUIDAndUsername(ctx, reservationUID, "alice")
	if err != nil || got.Id != reservationUID {
		t.Fatalf("GetByUIDAndUsername = (%+v, %v)", got, err)
	}
	tillDate := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	got, err = client.Rent(ctx, "alice", libraryUID, bookUID, tillDate)
	if err != nil || got.Status != domain.StatusRented {
		t.Fatalf("Rent = (%+v, %v)", got, err)
	}
	got, err = client.Return(ctx, reservationUID, "alice", tillDate)
	if err != nil || got.Status != domain.StatusReturned {
		t.Fatalf("Return = (%+v, %v)", got, err)
	}
	if err := client.Cancel(ctx, reservationUID, "alice"); err != nil {
		t.Fatalf("Cancel = %v", err)
	}
}

func TestClientMapsErrors(t *testing.T) {
	for _, check := range []struct {
		status     int
		want       error
		dependency bool
	}{
		{status: http.StatusNotFound, want: domain.ErrReservationNotFound},
		{status: http.StatusConflict, want: domain.ErrReservationNotRented},
		{status: http.StatusInternalServerError, dependency: true},
	} {
		transport := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: check.status,
				Body:       io.NopCloser(strings.NewReader(`{"message":"failed"}`)),
				Header:     make(http.Header),
			}, nil
		})
		client, err := NewClient("http://reservation-service:8070", &http.Client{Transport: transport}, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.GetByUIDAndUsername(context.Background(), uuid.New(), "alice")
		if check.want != nil && !errors.Is(err, check.want) {
			t.Fatalf("error = %v, want %v", err, check.want)
		}
		if check.dependency {
			var dependencyErr *usecase.DependencyError
			if !errors.As(err, &dependencyErr) {
				t.Fatalf("error = %v, want DependencyError", err)
			}
		}
	}
}

func TestNewClientRejectsInvalidURL(t *testing.T) {
	if _, err := NewClient("reservation-service:8070", nil, nil); err == nil {
		t.Fatal("expected invalid URL error")
	}
}
