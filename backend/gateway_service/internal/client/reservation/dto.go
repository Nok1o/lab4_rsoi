package reservation

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
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

func toReservation(source reservationResponse, username string) (domain.Reservation, error) {
	startDate, err := time.Parse(dateLayout, source.StartDate)
	if err != nil {
		return domain.Reservation{}, fmt.Errorf("parse reservation start date: %w", err)
	}
	tillDate, err := time.Parse(dateLayout, source.TillDate)
	if err != nil {
		return domain.Reservation{}, fmt.Errorf("parse reservation till date: %w", err)
	}
	return domain.Reservation{
		Id: source.UID, Username: username, BookId: source.BookUID, LibraryId: source.LibraryUID,
		Status: source.Status, StartDate: startDate, TillDate: tillDate,
	}, nil
}
