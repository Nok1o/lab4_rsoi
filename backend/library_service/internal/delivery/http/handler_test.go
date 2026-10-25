package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"libriary_system/library_service/internal/domain"
	"libriary_system/shared/pagination"
)

func TestListLibraries(t *testing.T) {
	libraryUID := uuid.MustParse("83575e12-7ce0-48ee-9931-51919ff3c9ee")
	stub := &libraryUseCaseStub{
		listLibraries: func(
			_ context.Context,
			city string,
			pageToken pagination.PageToken,
		) (pagination.Page[domain.Library], error) {
			if city != "Москва" {
				t.Fatalf("city = %q, want Москва", city)
			}
			if pageToken != (pagination.PageToken{Limit: 25, Offset: 50}) {
				t.Fatalf("pageToken = %+v", pageToken)
			}
			return pagination.Page[domain.Library]{
				Items: []domain.Library{{
					Id:      libraryUID,
					Name:    "Библиотека",
					City:    "Москва",
					Address: "Адрес",
				}},
				Total: 51,
			}, nil
		},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		stdhttp.MethodGet,
		"/api/v1/libraries?city=Москва&limit=25&offset=50",
		nil,
	)

	NewRouter(NewHandler(stub)).ServeHTTP(recorder, request)

	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, stdhttp.StatusOK)
	}
	var response pageResponse[libraryResponse]
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Total != 51 || len(response.Items) != 1 || response.Items[0].UID != libraryUID {
		t.Fatalf("response = %+v", response)
	}
}

func TestHealth(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/manage/health", nil)

	NewRouter(NewHandler(&libraryUseCaseStub{})).ServeHTTP(recorder, request)

	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, stdhttp.StatusOK)
	}
}

type libraryUseCaseStub struct {
	LibraryUseCase
	listLibraries func(
		context.Context,
		string,
		pagination.PageToken,
	) (pagination.Page[domain.Library], error)
}

func (stub *libraryUseCaseStub) ListLibrariesByCity(
	ctx context.Context,
	city string,
	pageToken pagination.PageToken,
) (pagination.Page[domain.Library], error) {
	return stub.listLibraries(ctx, city, pageToken)
}
