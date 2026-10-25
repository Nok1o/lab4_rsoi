package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	stdhttp "net/http"

	"libriary_system/rating_service/internal/domain"
	"libriary_system/rating_service/internal/usecase"
	"libriary_system/shared/log"
	"libriary_system/shared/username"
)

const maxRequestBodySize = 1 << 20

type RatingUseCase interface {
	GetByUsername(ctx context.Context, username string) (domain.Rating, error)
	Create(ctx context.Context, rating domain.Rating) error
	UpdateRating(ctx context.Context, rating domain.Rating) (domain.Rating, error)
	AddStars(ctx context.Context, username string, delta int) (domain.Rating, error)
}

type Handler struct {
	ratings RatingUseCase
}

func NewHandler(ratings RatingUseCase) *Handler {
	return &Handler{ratings: ratings}
}

func (handler *Handler) GetRating(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	rating, err := handler.ratings.GetByUsername(request.Context(), request.Header.Get("X-User-Name"))
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, toRatingResponse(rating))
}

func (handler *Handler) CreateRating(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	var body setRatingRequest
	if err := decodeJSON(w, request, &body); err != nil || body.Stars == nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: "request body must contain stars"})
		return
	}
	rating := domain.Rating{Username: request.Header.Get("X-User-Name"), StarsCount: *body.Stars}
	if err := handler.ratings.Create(request.Context(), rating); err != nil {
		handler.writeError(w, err)
		return
	}
	w.WriteHeader(stdhttp.StatusCreated)
}

func (handler *Handler) UpdateRating(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	var body setRatingRequest
	if err := decodeJSON(w, request, &body); err != nil || body.Stars == nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: "request body must contain stars"})
		return
	}
	rating, err := handler.ratings.UpdateRating(request.Context(), domain.Rating{
		Username: request.Header.Get("X-User-Name"), StarsCount: *body.Stars,
	})
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, toRatingResponse(rating))
}

func (handler *Handler) AddStars(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	var body addStarsRequest
	if err := decodeJSON(w, request, &body); err != nil || body.Delta == nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: "request body must contain delta"})
		return
	}
	rating, err := handler.ratings.AddStars(request.Context(), request.Header.Get("X-User-Name"), *body.Delta)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, toRatingResponse(rating))
}

func (handler *Handler) writeError(w stdhttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, username.ErrInvalid), errors.Is(err, usecase.InvalidStarsCountErr):
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: err.Error()})
	case errors.Is(err, domain.ErrRatingNotFound):
		writeJSON(w, stdhttp.StatusNotFound, errorResponse{Message: err.Error()})
	case errors.Is(err, domain.ErrRatingAlreadyExists):
		writeJSON(w, stdhttp.StatusConflict, errorResponse{Message: err.Error()})
	default:
		log.Error("rating request failed", "error", err)
		writeJSON(w, stdhttp.StatusInternalServerError, errorResponse{Message: "internal server error"})
	}
}

func decodeJSON(w stdhttp.ResponseWriter, request *stdhttp.Request, target any) error {
	request.Body = stdhttp.MaxBytesReader(w, request.Body, maxRequestBodySize)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeJSON(w stdhttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
