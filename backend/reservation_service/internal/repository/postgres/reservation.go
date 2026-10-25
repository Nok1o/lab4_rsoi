package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"libriary_system/reservation_service/internal/domain"
	"libriary_system/reservation_service/internal/usecase"
)

var _ usecase.ReservationRepository = (*ReservationPGRepo)(nil)

type ReservationPGRepo struct {
	pool *pgxpool.Pool
}

func NewReservationPGRepo(pool *pgxpool.Pool) *ReservationPGRepo {
	return &ReservationPGRepo{pool: pool}
}

func (repository *ReservationPGRepo) ListByUsername(ctx context.Context, username string) ([]domain.Reservation, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT reservation_uid, username, book_uid, library_uid, status, start_date, till_date
		FROM reservation
		WHERE username = $1
		ORDER BY start_date DESC, id DESC
	`, username)
	if err != nil {
		return nil, fmt.Errorf("list reservations: %w", err)
	}
	defer rows.Close()

	reservations := make([]domain.Reservation, 0)
	for rows.Next() {
		reservation, err := scanReservation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan reservation: %w", err)
		}
		reservations = append(reservations, reservation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reservations: %w", err)
	}
	return reservations, nil
}

func (repository *ReservationPGRepo) CountByUsernameAndStatus(
	ctx context.Context,
	username string,
	status domain.ReservationStatus,
) (int, error) {
	var count int
	if err := repository.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM reservation WHERE username = $1 AND status = $2
	`, username, status).Scan(&count); err != nil {
		return 0, fmt.Errorf("count reservations: %w", err)
	}
	return count, nil
}

func (repository *ReservationPGRepo) GetByUIDAndUsername(
	ctx context.Context,
	reservationUID uuid.UUID,
	username string,
) (domain.Reservation, error) {
	row := repository.pool.QueryRow(ctx, `
		SELECT reservation_uid, username, book_uid, library_uid, status, start_date, till_date
		FROM reservation
		WHERE reservation_uid = $1 AND username = $2
	`, reservationUID, username)
	reservation, err := scanReservation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Reservation{}, domain.ErrReservationNotFound
	}
	if err != nil {
		return domain.Reservation{}, fmt.Errorf("get reservation: %w", err)
	}
	return reservation, nil
}

func (repository *ReservationPGRepo) Create(ctx context.Context, reservation domain.Reservation) error {
	_, err := repository.pool.Exec(ctx, `
		INSERT INTO reservation (
			reservation_uid, username, book_uid, library_uid, status, start_date, till_date
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, reservation.Id, reservation.Username, reservation.BookId, reservation.LibraryId,
		reservation.Status, reservation.StartDate, reservation.TillDate)
	if err != nil {
		return fmt.Errorf("create reservation: %w", err)
	}
	return nil
}

func (repository *ReservationPGRepo) CompleteRental(
	ctx context.Context,
	reservationUID uuid.UUID,
	status domain.ReservationStatus,
) error {
	if status != domain.StatusReturned && status != domain.StatusExpired {
		return fmt.Errorf("invalid completed reservation status: %s", status)
	}
	result, err := repository.pool.Exec(ctx, `
		UPDATE reservation
		SET status = $2
		WHERE reservation_uid = $1 AND status = 'RENTED'
	`, reservationUID, status)
	if err != nil {
		return fmt.Errorf("complete rental: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrReservationNotRented
	}
	return nil
}

func (repository *ReservationPGRepo) Cancel(ctx context.Context, reservationUID uuid.UUID, username string) error {
	result, err := repository.pool.Exec(ctx, `
		DELETE FROM reservation
		WHERE reservation_uid = $1 AND username = $2 AND status = 'RENTED'
	`, reservationUID, username)
	if err != nil {
		return fmt.Errorf("cancel reservation: %w", err)
	}
	if result.RowsAffected() == 0 {
		if _, err := repository.GetByUIDAndUsername(ctx, reservationUID, username); err != nil {
			return err
		}
		return domain.ErrReservationNotRented
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanReservation(row rowScanner) (domain.Reservation, error) {
	var reservation domain.Reservation
	err := row.Scan(
		&reservation.Id,
		&reservation.Username,
		&reservation.BookId,
		&reservation.LibraryId,
		&reservation.Status,
		&reservation.StartDate,
		&reservation.TillDate,
	)
	return reservation, err
}
