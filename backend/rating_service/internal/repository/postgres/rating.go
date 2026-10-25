package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"libriary_system/rating_service/internal/domain"
	"libriary_system/rating_service/internal/usecase"
)

var _ usecase.RatingRepository = (*RatingPGRepo)(nil)

type RatingPGRepo struct {
	pool *pgxpool.Pool
}

func NewRatingPGRepo(pool *pgxpool.Pool) *RatingPGRepo {
	return &RatingPGRepo{pool: pool}
}

func (repository *RatingPGRepo) GetByUsername(
	ctx context.Context,
	username string,
) (domain.Rating, error) {
	var rating domain.Rating
	err := repository.pool.QueryRow(ctx, `
		SELECT username, stars
		FROM rating
		WHERE username = $1
	`, username).Scan(&rating.Username, &rating.StarsCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Rating{}, domain.ErrRatingNotFound
	}
	if err != nil {
		return domain.Rating{}, fmt.Errorf("get rating by username: %w", err)
	}

	return rating, nil
}

func (repository *RatingPGRepo) Create(
	ctx context.Context,
	rating domain.Rating,
) error {
	_, err := repository.pool.Exec(ctx, `
		INSERT INTO rating (username, stars)
		VALUES ($1, $2)
	`, rating.Username, rating.StarsCount)
	if err == nil {
		return nil
	}

	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
		return domain.ErrRatingAlreadyExists
	}
	return fmt.Errorf("create rating: %w", err)
}

func (repository *RatingPGRepo) UpdateStars(
	ctx context.Context,
	rating domain.Rating,
) (domain.Rating, error) {
	var updated domain.Rating
	err := repository.pool.QueryRow(ctx, `
		UPDATE rating
		SET stars = $2
		WHERE username = $1
		RETURNING username, stars
	`, rating.Username, rating.StarsCount).Scan(&updated.Username, &updated.StarsCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Rating{}, domain.ErrRatingNotFound
	}
	if err != nil {
		return domain.Rating{}, fmt.Errorf("update rating stars: %w", err)
	}

	return updated, nil
}

func (repository *RatingPGRepo) AddStars(
	ctx context.Context,
	username string,
	delta, minStars, maxStars int,
) (domain.Rating, error) {
	var updated domain.Rating
	err := repository.pool.QueryRow(ctx, `
		UPDATE rating
		SET stars = LEAST($4::integer, GREATEST($3::integer, stars + $2::integer))
		WHERE username = $1
		RETURNING username, stars
	`, username, delta, minStars, maxStars).Scan(&updated.Username, &updated.StarsCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Rating{}, domain.ErrRatingNotFound
	}
	if err != nil {
		return domain.Rating{}, fmt.Errorf("add rating stars: %w", err)
	}

	return updated, nil
}
