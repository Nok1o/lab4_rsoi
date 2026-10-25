package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
	"libriary_system/shared/log"
	"libriary_system/shared/pagination"
)

type GatewayUseCase interface {
	ListLibraries(ctx context.Context, city string, pageToken pagination.PageToken) (pagination.Page[domain.Library], error)
	ListBooks(ctx context.Context, libraryUID uuid.UUID, showAll bool, pageToken pagination.PageToken) (pagination.Page[domain.LibraryBook], error)
	GetRating(ctx context.Context, username string) (domain.Rating, error)
	ListReservations(ctx context.Context, username string) ([]usecase.ReservationDetails, error)
	Rent(ctx context.Context, username string, libraryUID, bookUID uuid.UUID, tillDate time.Time) (usecase.ReservationDetails, error)
	Return(ctx context.Context, reservationUID uuid.UUID, username string, condition domain.BookCondition, returnDate time.Time) error
}

type Handler struct {
	gateway GatewayUseCase
}

func NewHandler(gateway GatewayUseCase) *Handler {
	return &Handler{gateway: gateway}
}

func (handler *Handler) ListLibrariesByCity(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	page, pageToken, ok := parsePage(w, request)
	if !ok {
		return
	}
	result, err := handler.gateway.ListLibraries(request.Context(), request.URL.Query().Get("city"), pageToken)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, toLibraryPage(page, result))
}

func (handler *Handler) ListBooksByLibrary(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	libraryUID, ok := parseUID(w, request, "libraryUid")
	if !ok {
		return
	}
	page, pageToken, ok := parsePage(w, request)
	if !ok {
		return
	}
	showAll, ok := parseShowAll(w, request)
	if !ok {
		return
	}
	result, err := handler.gateway.ListBooks(request.Context(), libraryUID, showAll, pageToken)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, toBookPage(page, result))
}

func (handler *Handler) GetRating(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	rating, err := handler.gateway.GetRating(request.Context(), request.Header.Get("X-User-Name"))
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, ratingResponse{Stars: rating.StarsCount})
}

func (handler *Handler) ListReservations(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	details, err := handler.gateway.ListReservations(request.Context(), request.Header.Get("X-User-Name"))
	if err != nil {
		handler.writeError(w, err)
		return
	}
	response := make([]reservationResponse, 0, len(details))
	for _, item := range details {
		response = append(response, toReservationResponse(item))
	}
	writeJSON(w, stdhttp.StatusOK, response)
}

func (handler *Handler) TakeBook(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	var body takeBookRequest
	if err := decodeJSON(w, request, &body); err != nil {
		validationResponse(map[string]string{"body": "must be a single valid JSON object"}).writeResponse(w, stdhttp.StatusBadRequest)
		return
	}
	tillDate, err := time.Parse(dateLayout, body.TillDate)
	if err != nil {
		validationResponse(map[string]string{"tillDate": "must be YYYY-MM-DD"}).writeResponse(w, stdhttp.StatusBadRequest)
		return
	}
	details, err := handler.gateway.Rent(request.Context(), request.Header.Get("X-User-Name"), body.LibraryUID, body.BookUID, tillDate)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, toReservationResponse(details))
}

func (handler *Handler) ReturnBook(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	reservationUID, ok := parseUID(w, request, "reservationUid")
	if !ok {
		return
	}
	var body returnBookRequest
	if err := decodeJSON(w, request, &body); err != nil {
		validationResponse(map[string]string{"body": "must be a single valid JSON object"}).writeResponse(w, stdhttp.StatusBadRequest)
		return
	}
	returnDate, err := time.Parse(dateLayout, body.Date)
	if err != nil {
		validationResponse(map[string]string{"date": "must be YYYY-MM-DD"}).writeResponse(w, stdhttp.StatusBadRequest)
		return
	}
	if err := handler.gateway.Return(request.Context(), reservationUID, request.Header.Get("X-User-Name"), body.Condition, returnDate); err != nil {
		handler.writeError(w, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (handler *Handler) writeError(w stdhttp.ResponseWriter, err error) {
	if validationErr, ok := errors.AsType[*usecase.ValidationError](err); ok {
		validationResponse(validationErr.Fields).writeResponse(w, stdhttp.StatusBadRequest)
		return
	}
	switch {
	case errors.Is(err, domain.ErrLibraryNotFound), errors.Is(err, domain.ErrBookNotFound), errors.Is(err, domain.ErrReservationNotFound):
		newErrorResponse().message(err.Error()).writeResponse(w, stdhttp.StatusNotFound)
	case errors.Is(err, domain.ErrBookUnavailable), errors.Is(err, domain.ErrReservationNotRented), errors.Is(err, usecase.ErrRentalLimitReached):
		newErrorResponse().message(err.Error()).writeResponse(w, stdhttp.StatusConflict)
	default:
		log.Error("gateway request failed", "error", err)
		message := "upstream service error"
		if dependency, ok := errors.AsType[*usecase.DependencyError](err); ok && dependency.Service == "rating service" {
			// Keep the public error message required by the Lab4 API contract.
			message = "Bonus Service unavailable"
		}
		newErrorResponse().message(message).writeResponse(w, stdhttp.StatusServiceUnavailable)
	}
}

func parseUID(w stdhttp.ResponseWriter, request *stdhttp.Request, name string) (uuid.UUID, bool) {
	uid, err := uuid.Parse(mux.Vars(request)[name])
	if err != nil {
		validationResponse(map[string]string{name: "must be a valid UUID"}).writeResponse(w, stdhttp.StatusBadRequest)
		return uuid.Nil, false
	}
	return uid, true
}
