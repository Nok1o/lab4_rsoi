package domain

import (
	"errors"

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

var (
	ErrLibraryNotFound = errors.New("library not found")
	ErrBookNotFound    = errors.New("book not found in library")
	ErrBookUnavailable = errors.New("book is not available")
)
