package reservation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
	"libriary_system/pkg"
)

const maxResponseBodySize = 1 << 20

var _ usecase.ReservationService = (*Client)(nil)

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	breaker    *pkg.CircuitBreaker
}

type serviceError struct {
	statusCode int
	message    string
}

func (err *serviceError) Error() string {
	return fmt.Sprintf("reservation service returned status %d: %s", err.statusCode, err.message)
}

func (err *serviceError) StatusCode() int {
	return err.statusCode
}

func NewClient(rawBaseURL string, httpClient *http.Client, breaker *pkg.CircuitBreaker) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse reservation service URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("reservation service URL must use http or https scheme")
	}
	if baseURL.Host == "" {
		return nil, fmt.Errorf("reservation service URL must contain a host")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	if breaker == nil {
		breaker = pkg.NewCircuitBreaker(pkg.CircuitBreakerConfig{})
	}
	return &Client{baseURL: baseURL, httpClient: httpClient, breaker: breaker}, nil
}

func (client *Client) ListByUsername(ctx context.Context, username string) ([]domain.Reservation, error) {
	response, err := pkg.Do(client.breaker, func() ([]reservationResponse, error) {
		var response []reservationResponse
		err := client.do(ctx, http.MethodGet, "/api/v1/reservations", username, nil, http.StatusOK, &response)
		return response, err
	})
	if err != nil {
		return nil, mapError(err)
	}
	reservations := make([]domain.Reservation, 0, len(response))
	for _, item := range response {
		reservation, err := toReservation(item, username)
		if err != nil {
			return nil, dependencyError(err)
		}
		reservations = append(reservations, reservation)
	}
	return reservations, nil
}

func (client *Client) CountByUsernameAndStatus(
	ctx context.Context,
	username string,
	status domain.ReservationStatus,
) (int, error) {
	path := "/api/v1/reservations/count?status=" + url.QueryEscape(string(status))
	response, err := pkg.Do(client.breaker, func() (countResponse, error) {
		var response countResponse
		err := client.do(ctx, http.MethodGet, path, username, nil, http.StatusOK, &response)
		return response, err
	})
	if err != nil {
		return 0, mapError(err)
	}
	return response.Count, nil
}

func (client *Client) GetByUIDAndUsername(
	ctx context.Context,
	reservationUID uuid.UUID,
	username string,
) (domain.Reservation, error) {
	path := "/api/v1/reservations/" + reservationUID.String()
	response, err := pkg.Do(client.breaker, func() (reservationResponse, error) {
		var response reservationResponse
		err := client.do(ctx, http.MethodGet, path, username, nil, http.StatusOK, &response)
		return response, err
	})
	if err != nil {
		return domain.Reservation{}, mapError(err)
	}
	reservation, err := toReservation(response, username)
	if err != nil {
		return domain.Reservation{}, dependencyError(err)
	}
	return reservation, nil
}

func (client *Client) Rent(
	ctx context.Context,
	username string,
	libraryUID, bookUID uuid.UUID,
	tillDate time.Time,
) (domain.Reservation, error) {
	var response reservationResponse
	body := rentRequest{BookUID: bookUID, LibraryUID: libraryUID, TillDate: tillDate.Format(dateLayout)}
	if err := client.do(ctx, http.MethodPost, "/api/v1/reservations", username, body, http.StatusCreated, &response); err != nil {
		return domain.Reservation{}, mapError(err)
	}
	reservation, err := toReservation(response, username)
	if err != nil {
		return domain.Reservation{}, dependencyError(err)
	}
	return reservation, nil
}

func (client *Client) Return(
	ctx context.Context,
	reservationUID uuid.UUID,
	username string,
	returnDate time.Time,
) (domain.Reservation, error) {
	var response reservationResponse
	path := "/api/v1/reservations/" + reservationUID.String() + "/return"
	if err := client.do(ctx, http.MethodPost, path, username, returnRequest{Date: returnDate.Format(dateLayout)}, http.StatusOK, &response); err != nil {
		return domain.Reservation{}, mapError(err)
	}
	reservation, err := toReservation(response, username)
	if err != nil {
		return domain.Reservation{}, dependencyError(err)
	}
	return reservation, nil
}

func (client *Client) Cancel(ctx context.Context, reservationUID uuid.UUID, username string) error {
	path := "/api/v1/reservations/" + reservationUID.String()
	return mapError(client.do(ctx, http.MethodDelete, path, username, nil, http.StatusNoContent, nil))
}

func (client *Client) do(
	ctx context.Context,
	method, path, username string,
	body any,
	expectedStatus int,
	result any,
) error {
	endpoint := *client.baseURL
	pathParts := strings.SplitN(path, "?", 2)
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + pathParts[0]
	endpoint.RawQuery = ""
	if len(pathParts) == 2 {
		endpoint.RawQuery = pathParts[1]
	}
	endpoint.Fragment = ""

	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode reservation request: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), requestBody)
	if err != nil {
		return fmt.Errorf("create reservation request: %w", err)
	}
	request.Header.Set("X-User-Name", username)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call reservation service: %w", err)
	}
	defer response.Body.Close()

	limitedBody := io.LimitReader(response.Body, maxResponseBodySize)
	if response.StatusCode != expectedStatus {
		var serviceResponse errorResponse
		if err := json.NewDecoder(limitedBody).Decode(&serviceResponse); err != nil || serviceResponse.Message == "" {
			serviceResponse.Message = http.StatusText(response.StatusCode)
		}
		return &serviceError{statusCode: response.StatusCode, message: serviceResponse.Message}
	}
	if result == nil {
		_, _ = io.Copy(io.Discard, limitedBody)
		return nil
	}
	if err := json.NewDecoder(limitedBody).Decode(result); err != nil {
		return fmt.Errorf("decode reservation response: %w", err)
	}
	return nil
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if serviceErr, ok := errors.AsType[*serviceError](err); ok {
		switch serviceErr.statusCode {
		case http.StatusNotFound:
			return domain.ErrReservationNotFound
		case http.StatusConflict:
			return domain.ErrReservationNotRented
		}
	}
	return dependencyError(err)
}

func dependencyError(err error) error {
	return &usecase.DependencyError{Service: "reservation service", Cause: err}
}
