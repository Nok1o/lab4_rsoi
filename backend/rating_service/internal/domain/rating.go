package domain

import "errors"

type Rating struct {
	Username   string
	StarsCount int
}

var (
	ErrRatingNotFound      = errors.New("rating not found")
	ErrRatingAlreadyExists = errors.New("rating already exists")
)
