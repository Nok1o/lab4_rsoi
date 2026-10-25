package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	reservationhttp "libriary_system/reservation_service/internal/delivery/http"
	"libriary_system/reservation_service/internal/domain"
	"libriary_system/reservation_service/internal/usecase"
)

type memoryRepo struct {
	items map[uuid.UUID]domain.Reservation
}

func (repo *memoryRepo) ListByUsername(_ context.Context, username string) ([]domain.Reservation, error) {
	items := make([]domain.Reservation, 0)
	for _, item := range repo.items {
		if item.Username == username {
			items = append(items, item)
		}
	}
	return items, nil
}

func (repo *memoryRepo) CountByUsernameAndStatus(_ context.Context, username string, status domain.ReservationStatus) (int, error) {
	count := 0
	for _, item := range repo.items {
		if item.Username == username && item.Status == status {
			count++
		}
	}
	return count, nil
}

func (repo *memoryRepo) GetByUIDAndUsername(_ context.Context, uid uuid.UUID, username string) (domain.Reservation, error) {
	item, exists := repo.items[uid]
	if !exists || item.Username != username {
		return domain.Reservation{}, domain.ErrReservationNotFound
	}
	return item, nil
}

func (repo *memoryRepo) Create(_ context.Context, item domain.Reservation) error {
	repo.items[item.Id] = item
	return nil
}

func (repo *memoryRepo) CompleteRental(_ context.Context, uid uuid.UUID, status domain.ReservationStatus) error {
	item, exists := repo.items[uid]
	if !exists || item.Status != domain.StatusRented {
		return domain.ErrReservationNotRented
	}
	item.Status = status
	repo.items[uid] = item
	return nil
}

func (repo *memoryRepo) Cancel(ctx context.Context, uid uuid.UUID, username string) error {
	item, err := repo.GetByUIDAndUsername(ctx, uid, username)
	if err != nil {
		return err
	}
	if item.Status != domain.StatusRented {
		return domain.ErrReservationNotRented
	}
	delete(repo.items, uid)
	return nil
}

func TestReservationRoutes(t *testing.T) {
	repo := &memoryRepo{items: make(map[uuid.UUID]domain.Reservation)}
	handler := reservationhttp.NewHandler(usecase.NewReservationUseCase(repo))
	router := reservationhttp.NewRouter(handler)
	today := time.Now().UTC()
	tillDate := today.AddDate(0, 0, 1).Format("2006-01-02")
	returnDate := today.AddDate(0, 0, 2).Format("2006-01-02")
	libraryUID, bookUID := uuid.New(), uuid.New()

	call := func(method, path, username, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("X-User-Name", username)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	assertStatus := func(response *httptest.ResponseRecorder, want int) {
		t.Helper()
		if response.Code != want {
			t.Fatalf("status = %d, want %d; body = %s", response.Code, want, response.Body.String())
		}
	}

	assertStatus(call(http.MethodGet, "/manage/health", "", ""), http.StatusOK)
	assertStatus(call(http.MethodGet, "/api/v1/reservations", "alice", ""), http.StatusOK)
	assertStatus(call(http.MethodGet, "/api/v1/reservations/count?status=BAD", "alice", ""), http.StatusBadRequest)
	assertStatus(call(http.MethodPost, "/api/v1/reservations", "alice", `{"bookUid":"bad"}`), http.StatusBadRequest)

	body := `{"bookUid":"` + bookUID.String() + `","libraryUid":"` + libraryUID.String() + `","tillDate":"` + tillDate + `"}`
	created := call(http.MethodPost, "/api/v1/reservations", "alice", body)
	assertStatus(created, http.StatusCreated)
	var rental struct {
		UID    uuid.UUID                `json:"reservationUid"`
		Status domain.ReservationStatus `json:"status"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &rental); err != nil || rental.UID == uuid.Nil || rental.Status != domain.StatusRented {
		t.Fatalf("created rental = %+v, error = %v", rental, err)
	}
	path := "/api/v1/reservations/" + rental.UID.String()
	if got := created.Header().Get("Location"); got != path {
		t.Fatalf("Location = %q, want %q", got, path)
	}
	assertStatus(call(http.MethodGet, path, "bob", ""), http.StatusNotFound)
	assertStatus(call(http.MethodDelete, path, "bob", ""), http.StatusNotFound)
	assertStatus(call(http.MethodGet, path, "alice", ""), http.StatusOK)
	if response := call(http.MethodGet, "/api/v1/reservations/count?status=RENTED", "alice", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"count":1`) {
		t.Fatalf("count response = %d %s", response.Code, response.Body.String())
	}
	if response := call(http.MethodGet, "/api/v1/reservations", "alice", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), rental.UID.String()) {
		t.Fatalf("list response = %d %s", response.Code, response.Body.String())
	}
	returnPath := path + "/return"
	assertStatus(call(http.MethodPost, returnPath, "bob", `{"date":"`+returnDate+`"}`), http.StatusNotFound)
	returned := call(http.MethodPost, returnPath, "alice", `{"date":"`+returnDate+`"}`)
	assertStatus(returned, http.StatusOK)
	if !strings.Contains(returned.Body.String(), `"status":"EXPIRED"`) {
		t.Fatalf("return response = %s", returned.Body.String())
	}
	assertStatus(call(http.MethodPost, returnPath, "alice", `{"date":"`+returnDate+`"}`), http.StatusConflict)
	assertStatus(call(http.MethodDelete, path, "alice", ""), http.StatusConflict)
	second := call(http.MethodPost, "/api/v1/reservations", "alice", body)
	assertStatus(second, http.StatusCreated)
	cancelPath := second.Header().Get("Location")
	assertStatus(call(http.MethodDelete, cancelPath, "alice", ""), http.StatusNoContent)
	assertStatus(call(http.MethodGet, cancelPath, "alice", ""), http.StatusNotFound)
	assertStatus(call(http.MethodGet, "/api/v1/reservations/not-a-uuid", "alice", ""), http.StatusBadRequest)
}
