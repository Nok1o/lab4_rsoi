package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
	"libriary_system/pkg"
	"libriary_system/shared/pagination"
)

const maxResponseBodySize = 1 << 20

var _ usecase.LibraryService = (*Client)(nil)

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
	return fmt.Sprintf("library service returned status %d: %s", err.statusCode, err.message)
}

func (err *serviceError) StatusCode() int {
	return err.statusCode
}

func NewClient(rawBaseURL string, httpClient *http.Client, breaker *pkg.CircuitBreaker) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse library service URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("library service URL must use http or https scheme")
	}
	if baseURL.Host == "" {
		return nil, fmt.Errorf("library service URL must contain a host")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	if breaker == nil {
		breaker = pkg.NewCircuitBreaker(pkg.CircuitBreakerConfig{})
	}
	return &Client{baseURL: baseURL, httpClient: httpClient, breaker: breaker}, nil
}

func (client *Client) ListLibrariesByCity(
	ctx context.Context,
	city string,
	pageToken pagination.PageToken,
) (pagination.Page[domain.Library], error) {
	endpoint := client.endpoint("/api/v1/libraries")
	query := endpoint.Query()
	query.Set("city", city)
	setPageToken(query, pageToken)
	endpoint.RawQuery = query.Encode()

	response, err := pkg.Do(client.breaker, func() (pageResponse[libraryResponse], error) {
		var response pageResponse[libraryResponse]
		err := client.do(ctx, http.MethodGet, endpoint, nil, http.StatusOK, &response)
		return response, err
	})
	if err != nil {
		return pagination.Page[domain.Library]{}, dependencyError(err)
	}
	items := make([]domain.Library, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, toLibrary(item))
	}
	return pagination.Page[domain.Library]{Items: items, Total: response.Total}, nil
}

func (client *Client) ListBooksByLibrary(
	ctx context.Context,
	libraryUID uuid.UUID,
	showAll bool,
	pageToken pagination.PageToken,
) (pagination.Page[domain.LibraryBook], error) {
	endpoint := client.endpoint(fmt.Sprintf("/api/v1/libraries/%s/books", libraryUID))
	query := endpoint.Query()
	query.Set("showAll", strconv.FormatBool(showAll))
	setPageToken(query, pageToken)
	endpoint.RawQuery = query.Encode()

	response, err := pkg.Do(client.breaker, func() (pageResponse[libraryBookResponse], error) {
		var response pageResponse[libraryBookResponse]
		err := client.do(ctx, http.MethodGet, endpoint, nil, http.StatusOK, &response)
		return response, err
	})
	if err != nil {
		return pagination.Page[domain.LibraryBook]{}, dependencyError(err)
	}
	items := make([]domain.LibraryBook, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, toLibraryBook(item))
	}
	return pagination.Page[domain.LibraryBook]{Items: items, Total: response.Total}, nil
}

func (client *Client) GetLibraryByUID(ctx context.Context, libraryUID uuid.UUID) (domain.Library, error) {
	endpoint := client.endpoint(fmt.Sprintf("/api/v1/libraries/%s", libraryUID))
	response, err := pkg.Do(client.breaker, func() (libraryResponse, error) {
		var response libraryResponse
		err := client.do(ctx, http.MethodGet, endpoint, nil, http.StatusOK, &response)
		return response, err
	})
	if err != nil {
		if hasStatus(err, http.StatusNotFound) {
			return domain.Library{}, domain.ErrLibraryNotFound
		}
		return domain.Library{}, dependencyError(err)
	}
	return toLibrary(response), nil
}

func (client *Client) GetBookByUID(
	ctx context.Context,
	libraryUID, bookUID uuid.UUID,
) (domain.LibraryBook, error) {
	endpoint := client.bookEndpoint(libraryUID, bookUID)
	response, err := pkg.Do(client.breaker, func() (libraryBookResponse, error) {
		var response libraryBookResponse
		err := client.do(ctx, http.MethodGet, endpoint, nil, http.StatusOK, &response)
		return response, err
	})
	if err != nil {
		if hasStatus(err, http.StatusNotFound) {
			return domain.LibraryBook{}, domain.ErrBookNotFound
		}
		return domain.LibraryBook{}, dependencyError(err)
	}
	return toLibraryBook(response), nil
}

func (client *Client) ReserveBook(
	ctx context.Context,
	libraryUID, bookUID uuid.UUID,
) (domain.LibraryBook, error) {
	endpoint := client.bookEndpoint(libraryUID, bookUID)
	endpoint.Path += "/reserve"
	var response libraryBookResponse
	if err := client.do(ctx, http.MethodPost, endpoint, nil, http.StatusOK, &response); err != nil {
		switch {
		case hasStatus(err, http.StatusNotFound):
			return domain.LibraryBook{}, domain.ErrBookNotFound
		case hasStatus(err, http.StatusConflict):
			return domain.LibraryBook{}, domain.ErrBookUnavailable
		default:
			return domain.LibraryBook{}, dependencyError(err)
		}
	}
	return toLibraryBook(response), nil
}

func (client *Client) ReturnBook(
	ctx context.Context,
	libraryUID, bookUID uuid.UUID,
	condition domain.BookCondition,
) error {
	endpoint := client.bookEndpoint(libraryUID, bookUID)
	endpoint.Path += "/return"
	if err := client.do(ctx, http.MethodPost, endpoint, returnBookRequest{Condition: condition}, http.StatusNoContent, nil); err != nil {
		if hasStatus(err, http.StatusNotFound) {
			return domain.ErrBookNotFound
		}
		return dependencyError(err)
	}
	return nil
}

func (client *Client) bookEndpoint(libraryUID, bookUID uuid.UUID) *url.URL {
	return client.endpoint(fmt.Sprintf("/api/v1/libraries/%s/books/%s", libraryUID, bookUID))
}

func (client *Client) endpoint(path string) *url.URL {
	endpoint := *client.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return &endpoint
}

func (client *Client) do(
	ctx context.Context,
	method string,
	endpoint *url.URL,
	body any,
	expectedStatus int,
	responseTarget any,
) error {
	var requestBody io.Reader
	if body != nil {
		encodedBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode library service request: %w", err)
		}
		requestBody = bytes.NewReader(encodedBody)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), requestBody)
	if err != nil {
		return fmt.Errorf("create library service request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call library service: %w", err)
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
	if responseTarget == nil {
		_, _ = io.Copy(io.Discard, limitedBody)
		return nil
	}
	if err := json.NewDecoder(limitedBody).Decode(responseTarget); err != nil {
		return fmt.Errorf("decode library service response: %w", err)
	}
	return nil
}

func setPageToken(query url.Values, pageToken pagination.PageToken) {
	query.Set("limit", strconv.Itoa(pageToken.Limit))
	query.Set("offset", strconv.Itoa(pageToken.Offset))
}

func hasStatus(err error, status int) bool {
	var serviceErr *serviceError
	return errors.As(err, &serviceErr) && serviceErr.statusCode == status
}

func dependencyError(err error) error {
	return &usecase.DependencyError{Service: "library service", Cause: err}
}
