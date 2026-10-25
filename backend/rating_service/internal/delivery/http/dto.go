package http

import "libriary_system/rating_service/internal/domain"

type ratingResponse struct {
	Stars int `json:"stars"`
}

type setRatingRequest struct {
	Stars *int `json:"stars"`
}

type addStarsRequest struct {
	Delta *int `json:"delta"`
}

type errorResponse struct {
	Message string `json:"message"`
}

func toRatingResponse(rating domain.Rating) ratingResponse {
	return ratingResponse{Stars: rating.StarsCount}
}
