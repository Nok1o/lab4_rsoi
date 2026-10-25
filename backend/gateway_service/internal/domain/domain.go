package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type BookCondition string

const (
	ConditionExcellent BookCondition = "EXCELLENT"
	ConditionGood      BookCondition = "GOOD"
	ConditionBad       BookCondition = "BAD"
)

type Library struct {
	Id      uuid.UUID
	Name    string
	City    string
	Address string
}

type Book struct {
	Id        uuid.UUID
	Name      string
	Author    string
	Genre     string
	Condition BookCondition
}

type LibraryBook struct {
	Book
	AvailableCount int
}

type Rating struct {
	Username   string
	StarsCount int
}

type ReservationStatus string

const (
	StatusRented   ReservationStatus = "RENTED"
	StatusReturned ReservationStatus = "RETURNED"
	StatusExpired  ReservationStatus = "EXPIRED"
)

type Reservation struct {
	Id        uuid.UUID
	Username  string
	BookId    uuid.UUID
	LibraryId uuid.UUID
	Status    ReservationStatus
	StartDate time.Time
	TillDate  time.Time
}

var (
	ErrLibraryNotFound      = errors.New("library not found")
	ErrBookNotFound         = errors.New("book not found in library")
	ErrBookUnavailable      = errors.New("book is not available")
	ErrRatingNotFound       = errors.New("rating not found")
	ErrRatingAlreadyExists  = errors.New("rating already exists")
	ErrReservationNotFound  = errors.New("reservation not found")
	ErrReservationNotRented = errors.New("reservation is not rented")
)
