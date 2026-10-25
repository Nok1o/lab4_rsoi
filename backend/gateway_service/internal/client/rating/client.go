package rating

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

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
	"libriary_system/pkg"
)

const maxResponseBodySize = 1 << 20

var _ usecase.RatingService = (*Client)(nil)

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
	return fmt.Sprintf("rating service returned status %d: %s", err.statusCode, err.message)
}

func (err *serviceError) StatusCode() int {
	return err.statusCode
}

func NewClient(rawBaseURL string, httpClient *http.Client, breaker *pkg.CircuitBreaker) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse rating service URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("rating service URL must use http or https scheme")
	}
	if baseURL.Host == "" {
		return nil, fmt.Errorf("rating service URL must contain a host")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	if breaker == nil {
		breaker = pkg.NewCircuitBreaker(pkg.CircuitBreakerConfig{})
	}
	return &Client{baseURL: baseURL, httpClient: httpClient, breaker: breaker}, nil
}

func (client *Client) GetByUsername(ctx context.Context, username string) (domain.Rating, error) {
	response, err := pkg.Do(client.breaker, func() (ratingResponse, error) {
		var response ratingResponse
		err := client.do(ctx, http.MethodGet, username, nil, http.StatusOK, &response)
		return response, err
	})
	if err != nil {
		return domain.Rating{}, mapError(err)
	}
	return domain.Rating{Username: username, StarsCount: response.Stars}, nil
}

func (client *Client) Create(ctx context.Context, rating domain.Rating) error {
	return mapError(client.do(
		ctx,
		http.MethodPost,
		rating.Username,
		setRatingRequest{Stars: rating.StarsCount},
		http.StatusCreated,
		nil,
	))
}

func (client *Client) AddStars(ctx context.Context, username string, delta int) (domain.Rating, error) {
	var response ratingResponse
	if err := client.do(ctx, http.MethodPatch, username, addStarsRequest{Delta: delta}, http.StatusOK, &response); err != nil {
		return domain.Rating{}, mapError(err)
	}
	return domain.Rating{Username: username, StarsCount: response.Stars}, nil
}

func (client *Client) do(
	ctx context.Context,
	method, username string,
	body any,
	expectedStatus int,
	result any,
) error {
	endpoint := *client.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api/v1/rating"
	endpoint.RawQuery = ""
	endpoint.Fragment = ""

	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode rating request: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), requestBody)
	if err != nil {
		return fmt.Errorf("create rating request: %w", err)
	}
	request.Header.Set("X-User-Name", username)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call rating service: %w", err)
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
		return fmt.Errorf("decode rating response: %w", err)
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
			return domain.ErrRatingNotFound
		case http.StatusConflict:
			return domain.ErrRatingAlreadyExists
		}
	}
	return &usecase.DependencyError{Service: "rating service", Cause: err}
}
