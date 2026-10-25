package usecase

import (
	"libriary_system/rating_service/internal/domain"
	usernamevalidation "libriary_system/shared/username"
)

func validateRating(rating domain.Rating) error {
	if rating.StarsCount < minStars || rating.StarsCount > maxStars {
		return InvalidStarsCountErr
	}

	return usernamevalidation.Validate(rating.Username)
}
