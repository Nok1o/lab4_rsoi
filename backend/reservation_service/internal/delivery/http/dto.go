package http

import (
	"time"

	"github.com/google/uuid"

	"libriary_system/reservation_service/internal/domain"
)

const dateLayout = "2006-01-02"

type rentRequest struct {
	BookUID    uuid.UUID `json:"bookUid"`
	LibraryUID uuid.UUID `json:"libraryUid"`
	TillDate   string    `json:"tillDate"`
}

type returnRequest struct {
	Date string `json:"date"`
}

type reservationResponse struct {
	UID        uuid.UUID                `json:"reservationUid"`
	BookUID    uuid.UUID                `json:"bookUid"`
	LibraryUID uuid.UUID                `json:"libraryUid"`
	Status     domain.ReservationStatus `json:"status"`
	StartDate  string                   `json:"startDate"`
	TillDate   string                   `json:"tillDate"`
}

type countResponse struct {
	Count int `json:"count"`
}

type errorResponse struct {
	Message string `json:"message"`
}

func toReservationResponse(reservation domain.Reservation) reservationResponse {
	return reservationResponse{
		UID:        reservation.Id,
		BookUID:    reservation.BookId,
		LibraryUID: reservation.LibraryId,
		Status:     reservation.Status,
		StartDate:  reservation.StartDate.Format(dateLayout),
		TillDate:   reservation.TillDate.Format(dateLayout),
	}
}

func parseDate(raw string) (time.Time, error) {
	return time.Parse(dateLayout, raw)
}
