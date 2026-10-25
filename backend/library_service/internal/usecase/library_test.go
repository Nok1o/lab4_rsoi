package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"libriary_system/library_service/internal/domain"
	"libriary_system/shared/pagination"
)

var (
	testLibraryUID = uuid.MustParse("83575e12-7ce0-48ee-9931-51919ff3c9ee")
	testBookUID    = uuid.MustParse("f7cdc58f-2caf-4b15-9727-f89dcc629b27")
)

func TestLibraryUseCaseListLibrariesByCity(t *testing.T) {
	pageToken := pagination.PageToken{Limit: 10, Offset: 20}
	want := pagination.Page[domain.Library]{
		Items: []domain.Library{{Id: testLibraryUID, Name: "Library", City: "Moscow"}},
		Total: 21,
	}
	repository := &libraryRepositoryStub{
		listLibrariesByCity: func(
			_ context.Context,
			city string,
			gotPageToken pagination.PageToken,
		) (pagination.Page[domain.Library], error) {
			if city != "Moscow" {
				t.Fatalf("city = %q, want %q", city, "Moscow")
			}
			if gotPageToken != pageToken {
				t.Fatalf("pageToken = %+v, want %+v", gotPageToken, pageToken)
			}
			return want, nil
		},
	}

	got, err := NewLibraryUseCase(repository).ListLibrariesByCity(
		context.Background(),
		"  Moscow  ",
		pageToken,
	)
	if err != nil {
		t.Fatalf("ListLibrariesByCity() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListLibrariesByCity() = %+v, want %+v", got, want)
	}
}

func TestLibraryUseCaseListBooksByLibrary(t *testing.T) {
	pageToken := pagination.PageToken{Limit: 25, Offset: 0}
	want := pagination.Page[domain.LibraryBook]{
		Items: []domain.LibraryBook{{
			Book:           domain.Book{Id: testBookUID, Name: "Book"},
			AvailableCount: 1,
		}},
		Total: 1,
	}
	repository := &libraryRepositoryStub{
		listBooksByLibrary: func(
			_ context.Context,
			libraryUID uuid.UUID,
			showAll bool,
			gotPageToken pagination.PageToken,
		) (pagination.Page[domain.LibraryBook], error) {
			if libraryUID != testLibraryUID || !showAll || gotPageToken != pageToken {
				t.Fatalf(
					"arguments = (%s, %t, %+v), want (%s, true, %+v)",
					libraryUID,
					showAll,
					gotPageToken,
					testLibraryUID,
					pageToken,
				)
			}
			return want, nil
		},
	}

	got, err := NewLibraryUseCase(repository).ListBooksByLibrary(
		context.Background(),
		testLibraryUID,
		true,
		pageToken,
	)
	if err != nil {
		t.Fatalf("ListBooksByLibrary() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListBooksByLibrary() = %+v, want %+v", got, want)
	}
}

func TestLibraryUseCaseQueriesAndCommands(t *testing.T) {
	wantLibrary := domain.Library{Id: testLibraryUID, Name: "Library"}
	wantBook := domain.LibraryBook{Book: domain.Book{Id: testBookUID}, AvailableCount: 1}
	repository := &libraryRepositoryStub{
		getLibraryByUID: func(_ context.Context, libraryUID uuid.UUID) (domain.Library, error) {
			if libraryUID != testLibraryUID {
				t.Fatalf("libraryUID = %s, want %s", libraryUID, testLibraryUID)
			}
			return wantLibrary, nil
		},
		getBookByUID: func(_ context.Context, libraryUID, bookUID uuid.UUID) (domain.LibraryBook, error) {
			assertBookIDs(t, libraryUID, bookUID)
			return wantBook, nil
		},
		reserveBook: func(_ context.Context, libraryUID, bookUID uuid.UUID) (domain.LibraryBook, error) {
			assertBookIDs(t, libraryUID, bookUID)
			return wantBook, nil
		},
		returnBook: func(
			_ context.Context,
			libraryUID, bookUID uuid.UUID,
			condition domain.BookCondition,
		) error {
			assertBookIDs(t, libraryUID, bookUID)
			if condition != domain.ConditionGood {
				t.Fatalf("condition = %q, want %q", condition, domain.ConditionGood)
			}
			return nil
		},
	}
	useCase := NewLibraryUseCase(repository)

	gotLibrary, err := useCase.GetLibraryByUID(context.Background(), testLibraryUID)
	if err != nil || gotLibrary != wantLibrary {
		t.Fatalf("GetLibraryByUID() = (%+v, %v), want (%+v, nil)", gotLibrary, err, wantLibrary)
	}
	gotBook, err := useCase.GetBookByUID(context.Background(), testLibraryUID, testBookUID)
	if err != nil || !reflect.DeepEqual(gotBook, wantBook) {
		t.Fatalf("GetBookByUID() = (%+v, %v), want (%+v, nil)", gotBook, err, wantBook)
	}
	reservedBook, err := useCase.ReserveBook(context.Background(), testLibraryUID, testBookUID)
	if err != nil || !reflect.DeepEqual(reservedBook, wantBook) {
		t.Fatalf("ReserveBook() = (%+v, %v), want (%+v, nil)", reservedBook, err, wantBook)
	}
	if err := useCase.ReturnBook(
		context.Background(),
		testLibraryUID,
		testBookUID,
		domain.ConditionGood,
	); err != nil {
		t.Fatalf("ReturnBook() error = %v", err)
	}
}

func TestLibraryUseCaseRejectsInvalidInput(t *testing.T) {
	validPageToken := pagination.PageToken{Limit: 10, Offset: 0}
	tests := []struct {
		name      string
		wantField string
		call      func(*LibraryUseCase) error
	}{
		{
			name:      "empty city",
			wantField: "city",
			call: func(useCase *LibraryUseCase) error {
				_, err := useCase.ListLibrariesByCity(context.Background(), " ", validPageToken)
				return err
			},
		},
		{
			name:      "invalid limit",
			wantField: "limit",
			call: func(useCase *LibraryUseCase) error {
				_, err := useCase.ListLibrariesByCity(
					context.Background(),
					"Moscow",
					pagination.PageToken{Limit: 101},
				)
				return err
			},
		},
		{
			name:      "negative offset",
			wantField: "offset",
			call: func(useCase *LibraryUseCase) error {
				_, err := useCase.ListBooksByLibrary(
					context.Background(),
					testLibraryUID,
					false,
					pagination.PageToken{Limit: 10, Offset: -1},
				)
				return err
			},
		},
		{
			name:      "empty library UID",
			wantField: "libraryUid",
			call: func(useCase *LibraryUseCase) error {
				_, err := useCase.GetLibraryByUID(context.Background(), uuid.Nil)
				return err
			},
		},
		{
			name:      "empty book UID",
			wantField: "bookUid",
			call: func(useCase *LibraryUseCase) error {
				_, err := useCase.ReserveBook(context.Background(), testLibraryUID, uuid.Nil)
				return err
			},
		},
		{
			name:      "invalid condition",
			wantField: "condition",
			call: func(useCase *LibraryUseCase) error {
				return useCase.ReturnBook(
					context.Background(),
					testLibraryUID,
					testBookUID,
					domain.BookCondition("UNKNOWN"),
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call(NewLibraryUseCase(&libraryRepositoryStub{}))
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %v, want *ValidationError", err)
			}
			if _, ok := validationErr.Fields[test.wantField]; !ok {
				t.Fatalf("validation fields = %+v, want field %q", validationErr.Fields, test.wantField)
			}
		})
	}
}

func assertBookIDs(t *testing.T, libraryUID, bookUID uuid.UUID) {
	t.Helper()
	if libraryUID != testLibraryUID || bookUID != testBookUID {
		t.Fatalf(
			"IDs = (%s, %s), want (%s, %s)",
			libraryUID,
			bookUID,
			testLibraryUID,
			testBookUID,
		)
	}
}

type libraryRepositoryStub struct {
	listLibrariesByCity func(context.Context, string, pagination.PageToken) (pagination.Page[domain.Library], error)
	listBooksByLibrary  func(context.Context, uuid.UUID, bool, pagination.PageToken) (pagination.Page[domain.LibraryBook], error)
	getLibraryByUID     func(context.Context, uuid.UUID) (domain.Library, error)
	getBookByUID        func(context.Context, uuid.UUID, uuid.UUID) (domain.LibraryBook, error)
	reserveBook         func(context.Context, uuid.UUID, uuid.UUID) (domain.LibraryBook, error)
	returnBook          func(context.Context, uuid.UUID, uuid.UUID, domain.BookCondition) error
}

func (stub *libraryRepositoryStub) ListLibrariesByCity(
	ctx context.Context,
	city string,
	pageToken pagination.PageToken,
) (pagination.Page[domain.Library], error) {
	return stub.listLibrariesByCity(ctx, city, pageToken)
}

func (stub *libraryRepositoryStub) ListBooksByLibrary(
	ctx context.Context,
	libraryUID uuid.UUID,
	showAll bool,
	pageToken pagination.PageToken,
) (pagination.Page[domain.LibraryBook], error) {
	return stub.listBooksByLibrary(ctx, libraryUID, showAll, pageToken)
}

func (stub *libraryRepositoryStub) GetLibraryByUID(
	ctx context.Context,
	libraryUID uuid.UUID,
) (domain.Library, error) {
	return stub.getLibraryByUID(ctx, libraryUID)
}

func (stub *libraryRepositoryStub) GetBookByUID(
	ctx context.Context,
	libraryUID uuid.UUID,
	bookUID uuid.UUID,
) (domain.LibraryBook, error) {
	return stub.getBookByUID(ctx, libraryUID, bookUID)
}

func (stub *libraryRepositoryStub) ReserveBook(
	ctx context.Context,
	libraryUID uuid.UUID,
	bookUID uuid.UUID,
) (domain.LibraryBook, error) {
	return stub.reserveBook(ctx, libraryUID, bookUID)
}

func (stub *libraryRepositoryStub) ReturnBook(
	ctx context.Context,
	libraryUID uuid.UUID,
	bookUID uuid.UUID,
	condition domain.BookCondition,
) error {
	return stub.returnBook(ctx, libraryUID, bookUID, condition)
}
