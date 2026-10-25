package rating

type ratingResponse struct {
	Stars int `json:"stars"`
}

type setRatingRequest struct {
	Stars int `json:"stars"`
}

type addStarsRequest struct {
	Delta int `json:"delta"`
}

type errorResponse struct {
	Message string `json:"message"`
}
