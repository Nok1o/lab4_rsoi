package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"libriary_system/reservation_service/internal/domain"
	usernamevalidation "libriary_system/shared/username"
)

var (
	ErrInvalidReservation = errors.New("invalid reservation")
	ErrInvalidReturnDate  = errors.New("invalid return date")
)

type ReservationRepository interface {
	ListByUsername(ctx context.Context, username string) ([]domain.Reservation, error)
	CountByUsernameAndStatus(ctx context.Context, username string, status domain.ReservationStatus) (int, error)
	GetByUIDAndUsername(ctx context.Context, reservationUID uuid.UUID, username string) (domain.Reservation, error)
	Create(ctx context.Context, reservation domain.Reservation) error
	CompleteRental(ctx context.Context, reservationUID uuid.UUID, status domain.ReservationStatus) error
	Cancel(ctx context.Context, reservationUID uuid.UUID, username string) error
}

type ReservationUseCase struct {
	repository ReservationRepository
	now        func() time.Time
}

func NewReservationUseCase(repository ReservationRepository) *ReservationUseCase {
	return &ReservationUseCase{repository: repository, now: time.Now}
}

func (useCase *ReservationUseCase) Rent(
	ctx context.Context,
	username string,
	libraryUID, bookUID uuid.UUID,
	tillDate time.Time,
) (domain.Reservation, error) {
	if err := usernamevalidation.Validate(username); err != nil {
		return domain.Reservation{}, fmt.Errorf("%w: %w", ErrInvalidReservation, err)
	}
	if libraryUID == uuid.Nil || bookUID == uuid.Nil || tillDate.IsZero() {
		return domain.Reservation{}, ErrInvalidReservation
	}

	startDate := calendarDate(useCase.now().UTC())
	reservation := domain.Reservation{
		Id:        uuid.New(),
		Username:  username,
		BookId:    bookUID,
		LibraryId: libraryUID,
		Status:    domain.StatusRented,
		StartDate: startDate,
		TillDate:  calendarDate(tillDate),
	}
	if err := useCase.repository.Create(ctx, reservation); err != nil {
		return domain.Reservation{}, err
	}
	return reservation, nil
}

func (useCase *ReservationUseCase) ListByUsername(ctx context.Context, username string) ([]domain.Reservation, error) {
	if err := usernamevalidation.Validate(username); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidReservation, err)
	}
	return useCase.repository.ListByUsername(ctx, username)
}

func (useCase *ReservationUseCase) CountByUsernameAndStatus(
	ctx context.Context,
	username string,
	status domain.ReservationStatus,
) (int, error) {
	if err := usernamevalidation.Validate(username); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidReservation, err)
	}
	if !validStatus(status) {
		return 0, ErrInvalidReservation
	}
	return useCase.repository.CountByUsernameAndStatus(ctx, username, status)
}

func (useCase *ReservationUseCase) GetByUIDAndUsername(
	ctx context.Context,
	reservationUID uuid.UUID,
	username string,
) (domain.Reservation, error) {
	if reservationUID == uuid.Nil {
		return domain.Reservation{}, ErrInvalidReservation
	}
	if err := usernamevalidation.Validate(username); err != nil {
		return domain.Reservation{}, fmt.Errorf("%w: %w", ErrInvalidReservation, err)
	}
	return useCase.repository.GetByUIDAndUsername(ctx, reservationUID, username)
}

func (useCase *ReservationUseCase) Return(
	ctx context.Context,
	reservationUID uuid.UUID,
	username string,
	returnDate time.Time,
) (domain.Reservation, error) {
	if reservationUID == uuid.Nil {
		return domain.Reservation{}, ErrInvalidReservation
	}
	if err := usernamevalidation.Validate(username); err != nil {
		return domain.Reservation{}, fmt.Errorf("%w: %w", ErrInvalidReservation, err)
	}
	if returnDate.IsZero() {
		return domain.Reservation{}, ErrInvalidReturnDate
	}

	reservation, err := useCase.repository.GetByUIDAndUsername(ctx, reservationUID, username)
	if err != nil {
		return domain.Reservation{}, err
	}
	if reservation.Status != domain.StatusRented {
		return domain.Reservation{}, domain.ErrReservationNotRented
	}
	status := domain.StatusReturned
	if calendarDate(returnDate).After(calendarDate(reservation.TillDate)) {
		status = domain.StatusExpired
	}
	if err := useCase.repository.CompleteRental(ctx, reservationUID, status); err != nil {
		return domain.Reservation{}, err
	}
	reservation.Status = status
	return reservation, nil
}

// Cancel compensates a newly created rental; completed rentals are preserved.
func (useCase *ReservationUseCase) Cancel(ctx context.Context, reservationUID uuid.UUID, username string) error {
	if reservationUID == uuid.Nil {
		return ErrInvalidReservation
	}
	if err := usernamevalidation.Validate(username); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidReservation, err)
	}
	return useCase.repository.Cancel(ctx, reservationUID, username)
}

func calendarDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func validStatus(status domain.ReservationStatus) bool {
	return status == domain.StatusRented || status == domain.StatusReturned || status == domain.StatusExpired
}
