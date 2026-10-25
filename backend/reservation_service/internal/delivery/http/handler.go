package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	stdhttp "net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"libriary_system/reservation_service/internal/domain"
	"libriary_system/reservation_service/internal/usecase"
	"libriary_system/shared/log"
)

const maxRequestBodySize = 1 << 20

type ReservationUseCase interface {
	ListByUsername(ctx context.Context, username string) ([]domain.Reservation, error)
	CountByUsernameAndStatus(ctx context.Context, username string, status domain.ReservationStatus) (int, error)
	GetByUIDAndUsername(ctx context.Context, reservationUID uuid.UUID, username string) (domain.Reservation, error)
	Rent(ctx context.Context, username string, libraryUID, bookUID uuid.UUID, tillDate time.Time) (domain.Reservation, error)
	Return(ctx context.Context, reservationUID uuid.UUID, username string, returnDate time.Time) (domain.Reservation, error)
	Cancel(ctx context.Context, reservationUID uuid.UUID, username string) error
}

type Handler struct {
	reservations ReservationUseCase
}

func NewHandler(reservations ReservationUseCase) *Handler {
	return &Handler{reservations: reservations}
}

func (handler *Handler) ListReservations(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	reservations, err := handler.reservations.ListByUsername(request.Context(), request.Header.Get("X-User-Name"))
	if err != nil {
		handler.writeError(w, err)
		return
	}
	response := make([]reservationResponse, 0, len(reservations))
	for _, reservation := range reservations {
		response = append(response, toReservationResponse(reservation))
	}
	writeJSON(w, stdhttp.StatusOK, response)
}

func (handler *Handler) CountReservations(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	count, err := handler.reservations.CountByUsernameAndStatus(
		request.Context(),
		request.Header.Get("X-User-Name"),
		domain.ReservationStatus(request.URL.Query().Get("status")),
	)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, countResponse{Count: count})
}

func (handler *Handler) GetReservation(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	uid, ok := reservationUID(w, request)
	if !ok {
		return
	}
	reservation, err := handler.reservations.GetByUIDAndUsername(request.Context(), uid, request.Header.Get("X-User-Name"))
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, toReservationResponse(reservation))
}

func (handler *Handler) Rent(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	var body rentRequest
	if err := decodeJSON(w, request, &body); err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: "invalid request body"})
		return
	}
	tillDate, err := parseDate(body.TillDate)
	if err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: "tillDate must be YYYY-MM-DD"})
		return
	}
	reservation, err := handler.reservations.Rent(request.Context(), request.Header.Get("X-User-Name"), body.LibraryUID, body.BookUID, tillDate)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/reservations/"+reservation.Id.String())
	writeJSON(w, stdhttp.StatusCreated, toReservationResponse(reservation))
}

func (handler *Handler) Return(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	uid, ok := reservationUID(w, request)
	if !ok {
		return
	}
	var body returnRequest
	if err := decodeJSON(w, request, &body); err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: "invalid request body"})
		return
	}
	returnDate, err := parseDate(body.Date)
	if err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: "date must be YYYY-MM-DD"})
		return
	}
	reservation, err := handler.reservations.Return(request.Context(), uid, request.Header.Get("X-User-Name"), returnDate)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, toReservationResponse(reservation))
}

func (handler *Handler) Cancel(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	uid, ok := reservationUID(w, request)
	if !ok {
		return
	}
	if err := handler.reservations.Cancel(request.Context(), uid, request.Header.Get("X-User-Name")); err != nil {
		handler.writeError(w, err)
		return
	}
	w.WriteHeader(stdhttp.StatusNoContent)
}

func (handler *Handler) writeError(w stdhttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidReservation), errors.Is(err, usecase.ErrInvalidReturnDate):
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: err.Error()})
	case errors.Is(err, domain.ErrReservationNotFound):
		writeJSON(w, stdhttp.StatusNotFound, errorResponse{Message: err.Error()})
	case errors.Is(err, domain.ErrReservationNotRented):
		writeJSON(w, stdhttp.StatusConflict, errorResponse{Message: err.Error()})
	default:
		log.Error("reservation request failed", "error", err)
		writeJSON(w, stdhttp.StatusInternalServerError, errorResponse{Message: "internal server error"})
	}
}

func reservationUID(w stdhttp.ResponseWriter, request *stdhttp.Request) (uuid.UUID, bool) {
	uid, err := uuid.Parse(mux.Vars(request)["reservationUid"])
	if err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: "reservationUid must be a valid UUID"})
		return uuid.Nil, false
	}
	return uid, true
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
