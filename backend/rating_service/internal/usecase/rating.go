package usecase

import (
	"context"
	"fmt"

	"libriary_system/rating_service/internal/domain"
	usernamevalidation "libriary_system/shared/username"
)

const (
	minStars = 1
	maxStars = 100
)

type RatingRepository interface {
	GetByUsername(ctx context.Context, username string) (domain.Rating, error)
	Create(ctx context.Context, rating domain.Rating) error
	UpdateStars(ctx context.Context, rating domain.Rating) (domain.Rating, error)
	AddStars(ctx context.Context, username string, delta, minStars, maxStars int) (domain.Rating, error)
}

type RatingUseCase struct {
	ratingRepo RatingRepository
}

func NewRatingUseCase(ratingRepo RatingRepository) *RatingUseCase {
	return &RatingUseCase{ratingRepo: ratingRepo}
}

func (r *RatingUseCase) GetByUsername(ctx context.Context, username string) (domain.Rating, error) {
	if err := usernamevalidation.Validate(username); err != nil {
		return domain.Rating{}, fmt.Errorf("invalid username: %w", err)
	}

	return r.ratingRepo.GetByUsername(ctx, username)
}

func (r *RatingUseCase) Create(ctx context.Context, rating domain.Rating) error {
	if err := validateRating(rating); err != nil {
		return fmt.Errorf("invalid rating: %w", err)
	}

	return r.ratingRepo.Create(ctx, rating)
}

func (r *RatingUseCase) UpdateRating(ctx context.Context, rating domain.Rating) (domain.Rating, error) {
	if err := validateRating(rating); err != nil {
		return domain.Rating{}, fmt.Errorf("invalid rating: %w", err)
	}

	return r.ratingRepo.UpdateStars(ctx, rating)
}

func (r *RatingUseCase) AddStars(ctx context.Context, username string, stars int) (domain.Rating, error) {
	if err := usernamevalidation.Validate(username); err != nil {
		return domain.Rating{}, fmt.Errorf("invalid username: %w", err)
	}

	return r.ratingRepo.AddStars(ctx, username, clampDelta(stars), minStars, maxStars)
}

func clampDelta(delta int) int {
	if delta < -maxStars {
		return -maxStars
	}
	if delta > maxStars {
		return maxStars
	}
	return delta
}
