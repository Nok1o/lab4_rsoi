package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"libriary_system/library_service/internal/domain"
	"libriary_system/library_service/internal/usecase"
	"libriary_system/shared/pagination"
)

var _ usecase.LibraryRepository = (*LibraryPGRepo)(nil)

type LibraryPGRepo struct {
	pool *pgxpool.Pool
}

func NewLibraryPGRepo(pool *pgxpool.Pool) *LibraryPGRepo {
	return &LibraryPGRepo{pool: pool}
}

func (repository *LibraryPGRepo) ListLibrariesByCity(
	ctx context.Context,
	city string,
	pageToken pagination.PageToken,
) (pagination.Page[domain.Library], error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT
			library_uid,
			name,
			city,
			address,
			COUNT(*) OVER() AS total_count
		FROM library
		WHERE city = $1
		ORDER BY id
		LIMIT $2
		OFFSET $3
	`, city, pageToken.Limit, pageToken.Offset)
	if err != nil {
		return pagination.Page[domain.Library]{},
			fmt.Errorf("query libraries by city: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Library, 0, pageToken.Limit)
	var total int64

	for rows.Next() {
		var dto libraryDTO
		if err := rows.Scan(
			&dto.UID,
			&dto.Name,
			&dto.City,
			&dto.Address,
			&total,
		); err != nil {
			return pagination.Page[domain.Library]{},
				fmt.Errorf("scan library: %w", err)
		}

		items = append(items, dto.toDomain())
	}

	if err := rows.Err(); err != nil {
		return pagination.Page[domain.Library]{},
			fmt.Errorf("iterate libraries: %w", err)
	}

	if len(items) == 0 && pageToken.Offset > 0 {
		if err := repository.pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM library
			WHERE city = $1
		`, city).Scan(&total); err != nil {
			return pagination.Page[domain.Library]{},
				fmt.Errorf("count libraries by city: %w", err)
		}
	}

	return pagination.Page[domain.Library]{
		Items: items,
		Total: total,
	}, nil
}

func (repository *LibraryPGRepo) ListBooksByLibrary(
	ctx context.Context,
	libraryUID uuid.UUID,
	showAll bool,
	pageToken pagination.PageToken,
) (pagination.Page[domain.LibraryBook], error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT
			b.book_uid,
			b.name,
			b.author,
			b.genre,
			b.condition,
			lb.available_count,
			COUNT(*) OVER() AS total_count
		FROM library_books AS lb
		JOIN library AS l ON l.id = lb.library_id
		JOIN books AS b ON b.id = lb.book_id
		WHERE l.library_uid = $1
			AND ($2::boolean OR lb.available_count > 0)
		ORDER BY b.id
		LIMIT $3
		OFFSET $4
	`, libraryUID, showAll, pageToken.Limit, pageToken.Offset)
	if err != nil {
		return pagination.Page[domain.LibraryBook]{},
			fmt.Errorf("query books by library: %w", err)
	}
	defer rows.Close()

	items := make([]domain.LibraryBook, 0, pageToken.Limit)
	var total int64

	for rows.Next() {
		var dto libraryBookDTO
		if err := rows.Scan(
			&dto.UID,
			&dto.Name,
			&dto.Author,
			&dto.Genre,
			&dto.Condition,
			&dto.AvailableCount,
			&total,
		); err != nil {
			return pagination.Page[domain.LibraryBook]{},
				fmt.Errorf("scan library book: %w", err)
		}

		items = append(items, dto.toDomain())
	}

	if err := rows.Err(); err != nil {
		return pagination.Page[domain.LibraryBook]{},
			fmt.Errorf("iterate library books: %w", err)
	}

	if len(items) == 0 && pageToken.Offset > 0 {
		if err := repository.pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM library_books AS lb
			JOIN library AS l ON l.id = lb.library_id
			WHERE l.library_uid = $1
				AND ($2::boolean OR lb.available_count > 0)
		`, libraryUID, showAll).Scan(&total); err != nil {
			return pagination.Page[domain.LibraryBook]{},
				fmt.Errorf("count books by library: %w", err)
		}
	}

	return pagination.Page[domain.LibraryBook]{
		Items: items,
		Total: total,
	}, nil
}

func (repository *LibraryPGRepo) GetLibraryByUID(
	ctx context.Context,
	libraryUID uuid.UUID,
) (domain.Library, error) {
	var dto libraryDTO
	err := repository.pool.QueryRow(ctx, `
		SELECT library_uid, name, city, address
		FROM library
		WHERE library_uid = $1
	`, libraryUID).Scan(
		&dto.UID,
		&dto.Name,
		&dto.City,
		&dto.Address,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Library{}, domain.ErrLibraryNotFound
	}
	if err != nil {
		return domain.Library{}, fmt.Errorf("get library by UID: %w", err)
	}

	return dto.toDomain(), nil
}

func (repository *LibraryPGRepo) GetBookByUID(
	ctx context.Context,
	libraryUID uuid.UUID,
	bookUID uuid.UUID,
) (domain.LibraryBook, error) {
	var dto libraryBookDTO
	err := repository.pool.QueryRow(ctx, `
		SELECT
			b.book_uid,
			b.name,
			b.author,
			b.genre,
			b.condition,
			lb.available_count
		FROM library_books AS lb
		JOIN library AS l ON l.id = lb.library_id
		JOIN books AS b ON b.id = lb.book_id
		WHERE l.library_uid = $1
			AND b.book_uid = $2
	`, libraryUID, bookUID).Scan(
		&dto.UID,
		&dto.Name,
		&dto.Author,
		&dto.Genre,
		&dto.Condition,
		&dto.AvailableCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LibraryBook{}, domain.ErrBookNotFound
	}
	if err != nil {
		return domain.LibraryBook{}, fmt.Errorf("get book by UID: %w", err)
	}

	return dto.toDomain(), nil
}

func (repository *LibraryPGRepo) ReserveBook(
	ctx context.Context,
	libraryUID uuid.UUID,
	bookUID uuid.UUID,
) (domain.LibraryBook, error) {
	var dto libraryBookDTO
	err := repository.pool.QueryRow(ctx, `
		UPDATE library_books AS lb
		SET available_count = lb.available_count - 1
		FROM library AS l, books AS b
		WHERE l.id = lb.library_id
			AND b.id = lb.book_id
			AND l.library_uid = $1
			AND b.book_uid = $2
			AND lb.available_count > 0
		RETURNING
			b.book_uid,
			b.name,
			b.author,
			b.genre,
			b.condition,
			lb.available_count
	`, libraryUID, bookUID).Scan(
		&dto.UID,
		&dto.Name,
		&dto.Author,
		&dto.Genre,
		&dto.Condition,
		&dto.AvailableCount,
	)
	if err == nil {
		return dto.toDomain(), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.LibraryBook{}, fmt.Errorf("reserve book: %w", err)
	}

	if _, getErr := repository.GetBookByUID(ctx, libraryUID, bookUID); getErr != nil {
		return domain.LibraryBook{}, getErr
	}

	return domain.LibraryBook{}, domain.ErrBookUnavailable
}

func (repository *LibraryPGRepo) ReturnBook(
	ctx context.Context,
	libraryUID uuid.UUID,
	bookUID uuid.UUID,
	condition domain.BookCondition,
) error {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin return book transaction: %w", err)
	}
	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	var bookID int64
	err = transaction.QueryRow(ctx, `
		UPDATE books AS b
		SET condition = $3
		FROM library_books AS lb
		JOIN library AS l ON l.id = lb.library_id
		WHERE b.id = lb.book_id
			AND l.library_uid = $1
			AND b.book_uid = $2
		RETURNING b.id
	`, libraryUID, bookUID, condition).Scan(&bookID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrBookNotFound
	}
	if err != nil {
		return fmt.Errorf("update returned book condition: %w", err)
	}

	commandTag, err := transaction.Exec(ctx, `
		UPDATE library_books AS lb
		SET available_count = lb.available_count + 1
		FROM library AS l
		WHERE l.id = lb.library_id
			AND l.library_uid = $1
			AND lb.book_id = $2
	`, libraryUID, bookID)
	if err != nil {
		return fmt.Errorf("increment returned book availability: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return fmt.Errorf(
			"increment returned book availability: expected one row, affected %d",
			commandTag.RowsAffected(),
		)
	}

	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit return book transaction: %w", err)
	}

	return nil
}
