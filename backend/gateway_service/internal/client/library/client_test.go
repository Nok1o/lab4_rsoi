package library

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
	"libriary_system/shared/pagination"
)

func TestClientListLibrariesByCity(t *testing.T) {
	libraryUID := uuid.MustParse("83575e12-7ce0-48ee-9931-51919ff3c9ee")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/libraries" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("city") != "Москва" || query.Get("limit") != "10" || query.Get("offset") != "20" {
			t.Fatalf("query = %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"items":[{
				"libraryUid":"83575e12-7ce0-48ee-9931-51919ff3c9ee",
				"name":"Библиотека",
				"city":"Москва",
				"address":"Адрес"
			}],
			"total":21
		}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client(), nil)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	page, err := client.ListLibrariesByCity(
		context.Background(),
		"Москва",
		pagination.PageToken{Limit: 10, Offset: 20},
	)
	if err != nil {
		t.Fatalf("ListLibrariesByCity() error = %v", err)
	}
	if page.Total != 21 || len(page.Items) != 1 || page.Items[0].Id != libraryUID {
		t.Fatalf("page = %+v", page)
	}
}

func TestClientMapsLibraryErrors(t *testing.T) {
	for _, check := range []struct {
		name   string
		status int
		want   error
	}{
		{name: "unavailable", status: http.StatusConflict, want: domain.ErrBookUnavailable},
		{name: "dependency", status: http.StatusInternalServerError},
	} {
		t.Run(check.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(check.status)
				_, _ = w.Write([]byte(`{"message":"failed"}`))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, server.Client(), nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.ReserveBook(context.Background(), uuid.New(), uuid.New())
			if check.want != nil && !errors.Is(err, check.want) {
				t.Fatalf("error = %v, want %v", err, check.want)
			}
			if check.want == nil {
				var dependencyErr *usecase.DependencyError
				if !errors.As(err, &dependencyErr) {
					t.Fatalf("error = %v, want DependencyError", err)
				}
			}
		})
	}
}

func TestNewClientRejectsInvalidURL(t *testing.T) {
	if _, err := NewClient("library-service:8060", nil, nil); err == nil {
		t.Fatal("expected invalid URL error")
	}
}
