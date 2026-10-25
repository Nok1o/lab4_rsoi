package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ratinghttp "libriary_system/rating_service/internal/delivery/http"
	"libriary_system/rating_service/internal/domain"
	"libriary_system/rating_service/internal/usecase"
)

type memoryRepo struct {
	ratings map[string]domain.Rating
}

func (repo *memoryRepo) GetByUsername(_ context.Context, username string) (domain.Rating, error) {
	rating, exists := repo.ratings[username]
	if !exists {
		return domain.Rating{}, domain.ErrRatingNotFound
	}
	return rating, nil
}

func (repo *memoryRepo) Create(_ context.Context, rating domain.Rating) error {
	if _, exists := repo.ratings[rating.Username]; exists {
		return domain.ErrRatingAlreadyExists
	}
	repo.ratings[rating.Username] = rating
	return nil
}

func (repo *memoryRepo) UpdateStars(_ context.Context, rating domain.Rating) (domain.Rating, error) {
	if _, exists := repo.ratings[rating.Username]; !exists {
		return domain.Rating{}, domain.ErrRatingNotFound
	}
	repo.ratings[rating.Username] = rating
	return rating, nil
}

func (repo *memoryRepo) AddStars(_ context.Context, username string, delta, minStars, maxStars int) (domain.Rating, error) {
	rating, exists := repo.ratings[username]
	if !exists {
		return domain.Rating{}, domain.ErrRatingNotFound
	}
	rating.StarsCount += delta
	if rating.StarsCount < minStars {
		rating.StarsCount = minStars
	}
	if rating.StarsCount > maxStars {
		rating.StarsCount = maxStars
	}
	repo.ratings[username] = rating
	return rating, nil
}

func TestRatingRouter(t *testing.T) {
	repo := &memoryRepo{ratings: make(map[string]domain.Rating)}
	handler := ratinghttp.NewHandler(usecase.NewRatingUseCase(repo))
	router := ratinghttp.NewRouter(handler)

	checks := []struct {
		method, path, username, body string
		status                       int
		contains                     string
	}{
		{http.MethodGet, "/manage/health", "", "", http.StatusOK, `"status":"ok"`},
		{http.MethodGet, "/api/v1/rating", "alice", "", http.StatusNotFound, "rating not found"},
		{http.MethodPost, "/api/v1/rating", "alice", `{"stars":50}`, http.StatusCreated, ""},
		{http.MethodPost, "/api/v1/rating", "alice", `{"stars":50}`, http.StatusConflict, "rating already exists"},
		{http.MethodGet, "/api/v1/rating", "alice", "", http.StatusOK, `"stars":50`},
		{http.MethodPut, "/api/v1/rating", "alice", `{"stars":75}`, http.StatusOK, `"stars":75`},
		{http.MethodPatch, "/api/v1/rating", "alice", `{"delta":50}`, http.StatusOK, `"stars":100`},
		{http.MethodPatch, "/api/v1/rating", "alice", `{"delta":-200}`, http.StatusOK, `"stars":1`},
		{http.MethodPost, "/api/v1/rating", "bob", `{"stars":0}`, http.StatusBadRequest, "invalid stars count"},
		{http.MethodPost, "/api/v1/rating", "bob", `{"stars":20,"extra":1}`, http.StatusBadRequest, "request body"},
		{http.MethodPatch, "/api/v1/rating", "bob", `{}`, http.StatusBadRequest, "delta"},
		{http.MethodGet, "/api/v1/rating", "", "", http.StatusBadRequest, "invalid username"},
	}
	for _, check := range checks {
		request := httptest.NewRequest(check.method, check.path, strings.NewReader(check.body))
		request.Header.Set("X-User-Name", check.username)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != check.status || !strings.Contains(response.Body.String(), check.contains) {
			t.Errorf("%s %s body=%s: status=%d body=%q; want %d and %q", check.method, check.path, check.body, response.Code, response.Body.String(), check.status, check.contains)
		}
	}
	if _, err := repo.GetByUsername(context.Background(), "bob"); !errors.Is(err, domain.ErrRatingNotFound) {
		t.Fatal("invalid requests must not create a rating")
	}
}
