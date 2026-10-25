package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	gatewayhttp "libriary_system/gateway_service/internal/delivery/http"
	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
	"libriary_system/shared/pagination"
)

type stubGateway struct {
	t              *testing.T
	libraryUID     uuid.UUID
	bookUID        uuid.UUID
	reservationUID uuid.UUID
	returnCalls    int
	ratingErr      error
}

func (stub *stubGateway) details() usecase.ReservationDetails {
	rating := domain.Rating{Username: "alice", StarsCount: 75}
	return usecase.ReservationDetails{
		Reservation: domain.Reservation{
			Id: stub.reservationUID, Username: "alice", LibraryId: stub.libraryUID, BookId: stub.bookUID,
			Status:    domain.StatusRented,
			StartDate: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC),
			TillDate:  time.Date(2021, 10, 11, 0, 0, 0, 0, time.UTC),
		},
		Library: domain.Library{Id: stub.libraryUID, Name: "Library", City: "Москва", Address: "Street"},
		Book:    domain.LibraryBook{Book: domain.Book{Id: stub.bookUID, Name: "Book", Author: "Author", Genre: "Genre", Condition: domain.ConditionExcellent}, AvailableCount: 1},
		Rating:  &rating,
	}
}

func (stub *stubGateway) ListLibraries(_ context.Context, city string, pageToken pagination.PageToken) (pagination.Page[domain.Library], error) {
	if city != "Москва" || pageToken != (pagination.PageToken{Limit: 25, Offset: 25}) {
		stub.t.Errorf("ListLibraries args: city=%q token=%+v", city, pageToken)
	}
	return pagination.Page[domain.Library]{Items: []domain.Library{stub.details().Library}, Total: 50}, nil
}

func (stub *stubGateway) ListBooks(_ context.Context, libraryUID uuid.UUID, showAll bool, pageToken pagination.PageToken) (pagination.Page[domain.LibraryBook], error) {
	if libraryUID != stub.libraryUID || !showAll || pageToken != (pagination.PageToken{Limit: 10, Offset: 0}) {
		stub.t.Errorf("ListBooks args: library=%s showAll=%t token=%+v", libraryUID, showAll, pageToken)
	}
	return pagination.Page[domain.LibraryBook]{Items: []domain.LibraryBook{stub.details().Book}, Total: 1}, nil
}

func (stub *stubGateway) GetRating(_ context.Context, username string) (domain.Rating, error) {
	if stub.ratingErr != nil {
		return domain.Rating{}, stub.ratingErr
	}
	if username != "alice" {
		stub.t.Errorf("username = %q", username)
	}
	return domain.Rating{Username: username, StarsCount: 75}, nil
}

func (stub *stubGateway) ListReservations(_ context.Context, username string) ([]usecase.ReservationDetails, error) {
	if username != "alice" {
		stub.t.Errorf("username = %q", username)
	}
	details := stub.details()
	details.Rating = nil
	return []usecase.ReservationDetails{details}, nil
}

func (stub *stubGateway) Rent(_ context.Context, username string, libraryUID, bookUID uuid.UUID, tillDate time.Time) (usecase.ReservationDetails, error) {
	if username != "alice" || libraryUID != stub.libraryUID || bookUID != stub.bookUID || tillDate.Format("2006-01-02") != "2021-10-11" {
		stub.t.Errorf("Rent args: username=%q library=%s book=%s till=%s", username, libraryUID, bookUID, tillDate)
	}
	return stub.details(), nil
}

func (stub *stubGateway) Return(_ context.Context, reservationUID uuid.UUID, username string, condition domain.BookCondition, returnDate time.Time) error {
	if reservationUID != stub.reservationUID || username != "alice" || condition != domain.ConditionExcellent || returnDate.Format("2006-01-02") != "2021-10-11" {
		stub.t.Errorf("Return args: reservation=%s username=%q condition=%s date=%s", reservationUID, username, condition, returnDate)
	}
	stub.returnCalls++
	return nil
}

func TestUnavailableDependencyReturns503(t *testing.T) {
	ratingError := &usecase.DependencyError{Service: "rating service", Cause: errors.New("connection refused")}
	for _, test := range []struct {
		name    string
		err     error
		message string
	}{
		{name: "rating service", err: ratingError, message: "Bonus Service unavailable"},
		{name: "wrapped rating service", err: fmt.Errorf("get rating: %w", ratingError), message: "Bonus Service unavailable"},
		{name: "other dependency", err: &usecase.DependencyError{Service: "library service", Cause: errors.New("connection refused")}, message: "upstream service error"},
		{name: "unexpected error", err: errors.New("internal details"), message: "upstream service error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &stubGateway{t: t, ratingErr: test.err}
			router := gatewayhttp.NewRouter(gatewayhttp.NewHandler(stub))
			request := httptest.NewRequest(http.MethodGet, "/api/v1/rating", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("unavailable dependency status: %d %s", response.Code, response.Body.String())
			}
			var body struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Message != test.message {
				t.Fatalf("message = %q, want %q", body.Message, test.message)
			}
		})
	}
}

func TestGatewayRoutes(t *testing.T) {
	stub := &stubGateway{t: t, libraryUID: uuid.New(), bookUID: uuid.New(), reservationUID: uuid.New()}
	router := gatewayhttp.NewRouter(gatewayhttp.NewHandler(stub))
	call := func(method, path, username, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("X-User-Name", username)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	checkStatus := func(response *httptest.ResponseRecorder, status int) {
		t.Helper()
		if response.Code != status {
			t.Fatalf("status = %d, want %d; body=%s", response.Code, status, response.Body.String())
		}
	}
	checkStatus(call(http.MethodGet, "/manage/health", "", ""), http.StatusOK)
	libraries := call(http.MethodGet, "/api/v1/libraries?city=Москва&page=2&size=25", "", "")
	checkStatus(libraries, http.StatusOK)
	var page struct {
		Page     int   `json:"page"`
		PageSize int   `json:"pageSize"`
		Total    int64 `json:"totalElements"`
	}
	if err := json.Unmarshal(libraries.Body.Bytes(), &page); err != nil || page.Page != 2 || page.PageSize != 1 || page.Total != 50 {
		t.Fatalf("library page = %+v, error = %v", page, err)
	}
	books := call(http.MethodGet, "/api/v1/libraries/"+stub.libraryUID.String()+"/books?page=1&size=10&showAll=true", "", "")
	checkStatus(books, http.StatusOK)
	if !strings.Contains(books.Body.String(), `"condition":"EXCELLENT"`) {
		t.Fatalf("books=%s", books.Body.String())
	}
	checkStatus(call(http.MethodGet, "/api/v1/rating", "alice", ""), http.StatusOK)
	listed := call(http.MethodGet, "/api/v1/reservations", "alice", "")
	checkStatus(listed, http.StatusOK)
	if strings.Contains(listed.Body.String(), `"rating"`) {
		t.Fatalf("list must omit rating: %s", listed.Body.String())
	}
	body := `{"bookUid":"` + stub.bookUID.String() + `","libraryUid":"` + stub.libraryUID.String() + `","tillDate":"2021-10-11"}`
	rented := call(http.MethodPost, "/api/v1/reservations", "alice", body)
	checkStatus(rented, http.StatusOK)
	if !strings.Contains(rented.Body.String(), `"rating":{"stars":75}`) {
		t.Fatalf("rent response=%s", rented.Body.String())
	}
	returned := call(http.MethodPost, "/api/v1/reservations/"+stub.reservationUID.String()+"/return", "alice", `{"condition":"EXCELLENT","date":"2021-10-11"}`)
	checkStatus(returned, http.StatusNoContent)
	if returned.Body.Len() != 0 || stub.returnCalls != 1 {
		t.Fatalf("return response=%s calls=%d", returned.Body.String(), stub.returnCalls)
	}
}

func TestGatewayRejectsInvalidRequest(t *testing.T) {
	stub := &stubGateway{t: t, libraryUID: uuid.New(), bookUID: uuid.New(), reservationUID: uuid.New()}
	router := gatewayhttp.NewRouter(gatewayhttp.NewHandler(stub))
	for _, check := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/libraries?city=Москва&page=no", ""},
		{http.MethodGet, "/api/v1/libraries?city=Москва&size=101", ""},
		{http.MethodGet, "/api/v1/libraries/not-a-uuid/books", ""},
		{http.MethodGet, "/api/v1/libraries/" + stub.libraryUID.String() + "/books?showAll=no", ""},
		{http.MethodPost, "/api/v1/reservations", `{"bookUid":"bad"}`},
		{http.MethodPost, "/api/v1/reservations/" + stub.reservationUID.String() + "/return", `{"condition":"BAD","date":"bad"}`},
	} {
		request := httptest.NewRequest(check.method, check.path, strings.NewReader(check.body))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s %s: status=%d body=%s", check.method, check.path, response.Code, response.Body.String())
		}
	}
}
