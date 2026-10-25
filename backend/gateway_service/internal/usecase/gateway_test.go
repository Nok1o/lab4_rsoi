package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/shared/pagination"
)

type fakeLibrary struct {
	library       domain.Library
	book          domain.LibraryBook
	available     int
	returnCalls   int
	getBookErr    error
	getLibraryErr error
	reserveErr    error
	onReserve     func()
	returnErr     error
}

func (service *fakeLibrary) ListLibrariesByCity(_ context.Context, city string, _ pagination.PageToken) (pagination.Page[domain.Library], error) {
	if city == service.library.City {
		return pagination.Page[domain.Library]{Items: []domain.Library{service.library}, Total: 1}, nil
	}
	return pagination.Page[domain.Library]{Items: []domain.Library{}}, nil
}

func (service *fakeLibrary) ListBooksByLibrary(_ context.Context, libraryUID uuid.UUID, _ bool, _ pagination.PageToken) (pagination.Page[domain.LibraryBook], error) {
	if libraryUID != service.library.Id {
		return pagination.Page[domain.LibraryBook]{}, domain.ErrLibraryNotFound
	}
	return pagination.Page[domain.LibraryBook]{Items: []domain.LibraryBook{service.book}, Total: 1}, nil
}

func (service *fakeLibrary) GetLibraryByUID(_ context.Context, libraryUID uuid.UUID) (domain.Library, error) {
	if service.getLibraryErr != nil {
		return domain.Library{}, service.getLibraryErr
	}
	if libraryUID != service.library.Id {
		return domain.Library{}, domain.ErrLibraryNotFound
	}
	return service.library, nil
}

func (service *fakeLibrary) GetBookByUID(_ context.Context, libraryUID, bookUID uuid.UUID) (domain.LibraryBook, error) {
	if service.getBookErr != nil {
		return domain.LibraryBook{}, service.getBookErr
	}
	if libraryUID != service.library.Id || bookUID != service.book.Id {
		return domain.LibraryBook{}, domain.ErrBookNotFound
	}
	book := service.book
	book.AvailableCount = service.available
	return book, nil
}

func (service *fakeLibrary) ReserveBook(ctx context.Context, libraryUID, bookUID uuid.UUID) (domain.LibraryBook, error) {
	if service.onReserve != nil {
		service.onReserve()
	}
	if service.reserveErr != nil {
		return domain.LibraryBook{}, service.reserveErr
	}
	book, err := service.GetBookByUID(ctx, libraryUID, bookUID)
	if err != nil {
		return domain.LibraryBook{}, err
	}
	if service.available == 0 {
		return domain.LibraryBook{}, domain.ErrBookUnavailable
	}
	service.available--
	book.AvailableCount = service.available
	return book, nil
}

func (service *fakeLibrary) ReturnBook(_ context.Context, libraryUID, bookUID uuid.UUID, condition domain.BookCondition) error {
	service.returnCalls++
	if service.returnErr != nil {
		return service.returnErr
	}
	if libraryUID != service.library.Id || bookUID != service.book.Id {
		return domain.ErrBookNotFound
	}
	service.available++
	service.book.Condition = condition
	return nil
}

type fakeRating struct {
	items  map[string]domain.Rating
	getErr error
	addErr error
}

func (service *fakeRating) GetByUsername(_ context.Context, username string) (domain.Rating, error) {
	if service.getErr != nil {
		return domain.Rating{}, service.getErr
	}
	rating, exists := service.items[username]
	if !exists {
		return domain.Rating{}, domain.ErrRatingNotFound
	}
	return rating, nil
}

func (service *fakeRating) Create(_ context.Context, rating domain.Rating) error {
	if _, exists := service.items[rating.Username]; exists {
		return domain.ErrRatingAlreadyExists
	}
	service.items[rating.Username] = rating
	return nil
}

func (service *fakeRating) AddStars(_ context.Context, username string, delta int) (domain.Rating, error) {
	if service.addErr != nil {
		return domain.Rating{}, service.addErr
	}
	rating, exists := service.items[username]
	if !exists {
		return domain.Rating{}, domain.ErrRatingNotFound
	}
	rating.StarsCount += delta
	if rating.StarsCount < 1 {
		rating.StarsCount = 1
	}
	if rating.StarsCount > 100 {
		rating.StarsCount = 100
	}
	service.items[username] = rating
	return rating, nil
}

type fakeReturnQueue struct {
	jobs []ReturnJob
	err  error
}

func (queue *fakeReturnQueue) Enqueue(_ context.Context, job ReturnJob) error {
	if queue.err != nil {
		return queue.err
	}
	queue.jobs = append(queue.jobs, job)
	return nil
}

type fakeReservation struct {
	items       map[uuid.UUID]domain.Reservation
	rentErr     error
	listErr     error
	cancelErr   error
	cancelCalls int
}

func (service *fakeReservation) ListByUsername(_ context.Context, username string) ([]domain.Reservation, error) {
	if service.listErr != nil {
		return nil, service.listErr
	}
	items := make([]domain.Reservation, 0)
	for _, item := range service.items {
		if item.Username == username {
			items = append(items, item)
		}
	}
	return items, nil
}

func (service *fakeReservation) CountByUsernameAndStatus(_ context.Context, username string, status domain.ReservationStatus) (int, error) {
	count := 0
	for _, item := range service.items {
		if item.Username == username && item.Status == status {
			count++
		}
	}
	return count, nil
}

func (service *fakeReservation) GetByUIDAndUsername(_ context.Context, reservationUID uuid.UUID, username string) (domain.Reservation, error) {
	item, exists := service.items[reservationUID]
	if !exists || item.Username != username {
		return domain.Reservation{}, domain.ErrReservationNotFound
	}
	return item, nil
}

func (service *fakeReservation) Rent(_ context.Context, username string, libraryUID, bookUID uuid.UUID, tillDate time.Time) (domain.Reservation, error) {
	if service.rentErr != nil {
		return domain.Reservation{}, service.rentErr
	}
	item := domain.Reservation{
		Id: uuid.New(), Username: username, LibraryId: libraryUID, BookId: bookUID,
		Status: domain.StatusRented, StartDate: time.Now().UTC(), TillDate: tillDate,
	}
	service.items[item.Id] = item
	return item, nil
}

func (service *fakeReservation) Cancel(ctx context.Context, reservationUID uuid.UUID, username string) error {
	service.cancelCalls++
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if service.cancelErr != nil {
		return service.cancelErr
	}
	item, err := service.GetByUIDAndUsername(ctx, reservationUID, username)
	if err != nil {
		return err
	}
	if item.Status != domain.StatusRented {
		return domain.ErrReservationNotRented
	}
	delete(service.items, reservationUID)
	return nil
}

func (service *fakeReservation) Return(_ context.Context, reservationUID uuid.UUID, username string, returnDate time.Time) (domain.Reservation, error) {
	item, err := service.GetByUIDAndUsername(context.Background(), reservationUID, username)
	if err != nil {
		return domain.Reservation{}, err
	}
	if item.Status != domain.StatusRented {
		return domain.Reservation{}, domain.ErrReservationNotRented
	}
	item.Status = domain.StatusReturned
	if returnDate.After(item.TillDate) {
		item.Status = domain.StatusExpired
	}
	service.items[item.Id] = item
	return item, nil
}

func newTestGateway() (*GatewayUseCase, *fakeLibrary, *fakeRating, *fakeReservation) {
	gateway, library, rating, reservation, _ := newTestGatewayWithQueue()
	return gateway, library, rating, reservation
}

func newTestGatewayWithQueue() (*GatewayUseCase, *fakeLibrary, *fakeRating, *fakeReservation, *fakeReturnQueue) {
	library := &fakeLibrary{
		library:   domain.Library{Id: uuid.New(), Name: "Library", City: "Москва", Address: "Address"},
		book:      domain.LibraryBook{Book: domain.Book{Id: uuid.New(), Name: "Book", Condition: domain.ConditionExcellent}},
		available: 1,
	}
	rating := &fakeRating{items: make(map[string]domain.Rating)}
	reservation := &fakeReservation{items: make(map[uuid.UUID]domain.Reservation)}
	returnQueue := &fakeReturnQueue{}
	return NewGatewayUseCase(library, rating, reservation, returnQueue), library, rating, reservation, returnQueue
}

func TestGetRatingDoesNotCreateMissingRating(t *testing.T) {
	gateway, _, rating, _ := newTestGateway()

	got, err := gateway.GetRating(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetRating() error = %v", err)
	}
	if got.Username != "alice" || got.StarsCount != initialStars {
		t.Fatalf("GetRating() = %+v", got)
	}
	if len(rating.items) != 0 {
		t.Fatalf("GET created rating: %+v", rating.items)
	}
}

func TestGatewayRentAndReturn(t *testing.T) {
	gateway, library, rating, reservationService := newTestGateway()
	ctx := context.Background()
	// The supplied Postman collection uses historical dates. The API contract
	// compares the return date with tillDate, not with the current clock.
	tillDate := time.Date(2021, 10, 11, 0, 0, 0, 0, time.UTC)
	details, err := gateway.Rent(ctx, "alice", library.library.Id, library.book.Id, tillDate)
	if err != nil || details.Reservation.Status != domain.StatusRented || details.Rating == nil || details.Rating.StarsCount != 75 {
		t.Fatalf("Rent() = (%+v, %v)", details, err)
	}
	if library.available != 0 || len(reservationService.items) != 1 {
		t.Fatalf("state after rent: available=%d, reservations=%d", library.available, len(reservationService.items))
	}
	items, err := gateway.ListReservations(ctx, "alice")
	if err != nil || len(items) != 1 || items[0].Book.Id != library.book.Id || items[0].Library.Id != library.library.Id {
		t.Fatalf("ListReservations() = (%+v, %v)", items, err)
	}
	if err := gateway.Return(ctx, details.Reservation.Id, "alice", domain.ConditionExcellent, tillDate); err != nil {
		t.Fatalf("Return() error = %v", err)
	}
	if library.available != 1 || rating.items["alice"].StarsCount != 76 {
		t.Fatalf("state after return: available=%d, rating=%d", library.available, rating.items["alice"].StarsCount)
	}
	if err := gateway.Return(ctx, details.Reservation.Id, "alice", domain.ConditionExcellent, tillDate); !errors.Is(err, domain.ErrReservationNotRented) {
		t.Fatalf("repeated Return() error = %v", err)
	}
	if library.returnCalls != 1 {
		t.Fatalf("library return calls = %d, want 1", library.returnCalls)
	}
}

func TestReturnPenalties(t *testing.T) {
	for _, check := range []struct {
		name       string
		returnDate time.Time
		condition  domain.BookCondition
		wantStars  int
	}{
		{"late", time.Date(2021, 10, 12, 0, 0, 0, 0, time.UTC), domain.ConditionExcellent, 65},
		{"condition changed", time.Date(2021, 10, 11, 0, 0, 0, 0, time.UTC), domain.ConditionBad, 65},
		{"both", time.Date(2021, 10, 12, 0, 0, 0, 0, time.UTC), domain.ConditionBad, 55},
	} {
		t.Run(check.name, func(t *testing.T) {
			gateway, library, rating, _ := newTestGateway()
			tillDate := time.Date(2021, 10, 11, 0, 0, 0, 0, time.UTC)
			details, err := gateway.Rent(context.Background(), "alice", library.library.Id, library.book.Id, tillDate)
			if err != nil {
				t.Fatal(err)
			}
			if err := gateway.Return(context.Background(), details.Reservation.Id, "alice", check.condition, check.returnDate); err != nil {
				t.Fatal(err)
			}
			if got := rating.items["alice"].StarsCount; got != check.wantStars {
				t.Fatalf("stars = %d, want %d", got, check.wantStars)
			}
		})
	}
}

func TestRentalLimitAndCompensation(t *testing.T) {
	gateway, library, rating, reservations := newTestGateway()
	rating.items["alice"] = domain.Rating{Username: "alice", StarsCount: 1}
	reservations.items[uuid.New()] = domain.Reservation{Username: "alice", Status: domain.StatusRented}
	tillDate := time.Date(2021, 10, 11, 0, 0, 0, 0, time.UTC)
	if _, err := gateway.Rent(context.Background(), "alice", library.library.Id, library.book.Id, tillDate); !errors.Is(err, ErrRentalLimitReached) {
		t.Fatalf("Rent() error = %v, want limit reached", err)
	}
	if library.available != 1 {
		t.Fatal("limit must not change availability")
	}
	reservations.items = make(map[uuid.UUID]domain.Reservation)
	reservations.rentErr = errors.New("database unavailable")
	if _, err := gateway.Rent(context.Background(), "alice", library.library.Id, library.book.Id, tillDate); err == nil {
		t.Fatal("Rent() error = nil, want database error")
	}
	if library.available != 1 || library.returnCalls != 0 || reservations.cancelCalls != 0 || len(reservations.items) != 0 {
		t.Fatal("failed creation must not change the library or trigger compensation")
	}
	reservations.rentErr = nil
	library.reserveErr = domain.ErrBookUnavailable
	library.onReserve = func() {
		if len(reservations.items) != 1 {
			t.Fatal("reservation must exist before decrementing availability")
		}
	}
	if _, err := gateway.Rent(context.Background(), "alice", library.library.Id, library.book.Id, tillDate); !errors.Is(err, domain.ErrBookUnavailable) {
		t.Fatalf("failed reserve error = %v", err)
	}
	if len(reservations.items) != 0 || reservations.cancelCalls != 1 || library.available != 1 {
		t.Fatal("new reservation was not cancelled")
	}
}

func TestRentCompensatesAfterClientCancellation(t *testing.T) {
	gateway, library, _, reservations := newTestGateway()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	library.onReserve = cancel
	library.reserveErr = context.Canceled
	_, err := gateway.Rent(ctx, "alice", library.library.Id, library.book.Id, time.Now())
	if !errors.Is(err, context.Canceled) || reservations.cancelCalls != 1 || len(reservations.items) != 0 {
		t.Fatalf("compensation after cancellation: err=%v, calls=%d, items=%v", err, reservations.cancelCalls, reservations.items)
	}
}

func TestRentReportsFailedCompensation(t *testing.T) {
	gateway, library, _, reservations := newTestGateway()
	library.reserveErr = &DependencyError{Service: "library", Cause: errors.New("unavailable")}
	reservations.cancelErr = errors.New("reservation service unavailable")
	_, err := gateway.Rent(context.Background(), "alice", library.library.Id, library.book.Id, time.Now())
	if err == nil || reservations.cancelCalls != 1 || len(reservations.items) != 1 {
		t.Fatalf("failed compensation: %v", err)
	}
}

func TestListReservationsFallsBackIndependently(t *testing.T) {
	for _, test := range []struct {
		name                  string
		libraryDown, bookDown bool
	}{
		{"library", true, false}, {"book", false, true}, {"both", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			gateway, library, _, reservations := newTestGateway()
			reservation := domain.Reservation{Id: uuid.New(), Username: "alice", LibraryId: library.library.Id, BookId: library.book.Id, Status: domain.StatusRented}
			reservations.items[reservation.Id] = reservation
			unavailable := &DependencyError{Service: "library", Cause: errors.New("connection refused")}
			if test.libraryDown {
				library.getLibraryErr = unavailable
			}
			if test.bookDown {
				library.getBookErr = unavailable
			}
			items, err := gateway.ListReservations(context.Background(), "alice")
			if err != nil || len(items) != 1 {
				t.Fatalf("ListReservations = (%+v, %v)", items, err)
			}
			item := items[0]
			if item.Library.Id != reservation.LibraryId || item.Book.Id != reservation.BookId {
				t.Fatal("fallback lost identifiers")
			}
			if (item.Library.Name == "") != test.libraryDown || (item.Book.Name == "") != test.bookDown {
				t.Fatalf("wrong fallback: %+v", item)
			}
		})
	}
}

func TestListReservationsDoesNotHidePermanentOrPrimaryFailures(t *testing.T) {
	gateway, library, _, reservations := newTestGateway()
	reservation := domain.Reservation{Id: uuid.New(), Username: "alice", LibraryId: library.library.Id, BookId: library.book.Id}
	reservations.items[reservation.Id] = reservation
	library.getBookErr = domain.ErrBookNotFound
	if _, err := gateway.ListReservations(context.Background(), "alice"); !errors.Is(err, domain.ErrBookNotFound) {
		t.Fatalf("permanent error hidden: %v", err)
	}
	reservations.listErr = &DependencyError{Service: "reservation", Cause: errors.New("unavailable")}
	if _, err := gateway.ListReservations(context.Background(), "alice"); !errors.Is(err, reservations.listErr) {
		t.Fatalf("primary failure hidden: %v", err)
	}
}

func TestReturnQueuesBookRestoreWhenLibraryIsUnavailable(t *testing.T) {
	gateway, library, rating, _, returnQueue := newTestGatewayWithQueue()
	tillDate := time.Date(2021, 10, 11, 0, 0, 0, 0, time.UTC)
	details, err := gateway.Rent(context.Background(), "alice", library.library.Id, library.book.Id, tillDate)
	if err != nil {
		t.Fatal(err)
	}

	library.getBookErr = &DependencyError{Service: "library", Cause: errors.New("library unavailable")}
	if err := gateway.Return(
		context.Background(),
		details.Reservation.Id,
		"alice",
		domain.ConditionExcellent,
		tillDate,
	); err != nil {
		t.Fatalf("Return() error = %v, want successful deferred return", err)
	}

	if len(returnQueue.jobs) != 1 {
		t.Fatalf("queued jobs = %d, want 1", len(returnQueue.jobs))
	}
	job := returnQueue.jobs[0]
	if job.Stage != ReturnStageRestoreBook ||
		job.ReservationUID != details.Reservation.Id ||
		job.LibraryUID != library.library.Id ||
		job.BookUID != library.book.Id ||
		job.Username != "alice" ||
		job.OriginalCondition != "" ||
		job.ReturnedCondition != domain.ConditionExcellent {
		t.Fatalf("queued job = %+v", job)
	}
	if rating.items["alice"].StarsCount != initialStars {
		t.Fatalf("rating changed before queued book restore: %+v", rating.items["alice"])
	}
}

func TestReturnQueuesOnlyRatingAfterBookWasRestored(t *testing.T) {
	gateway, library, rating, _, returnQueue := newTestGatewayWithQueue()
	tillDate := time.Date(2021, 10, 11, 0, 0, 0, 0, time.UTC)
	details, err := gateway.Rent(context.Background(), "alice", library.library.Id, library.book.Id, tillDate)
	if err != nil {
		t.Fatal(err)
	}

	rating.getErr = errors.New("rating unavailable")
	rating.addErr = &DependencyError{Service: "rating", Cause: errors.New("rating unavailable")}
	if err := gateway.Return(
		context.Background(),
		details.Reservation.Id,
		"alice",
		domain.ConditionExcellent,
		tillDate,
	); err != nil {
		t.Fatalf("Return() error = %v, want successful deferred return", err)
	}

	if library.available != 1 {
		t.Fatalf("available books = %d, want 1", library.available)
	}
	if len(returnQueue.jobs) != 1 {
		t.Fatalf("queued jobs = %d, want 1", len(returnQueue.jobs))
	}
	if job := returnQueue.jobs[0]; job.Stage != ReturnStageUpdateRating {
		t.Fatalf("queued stage = %s, want %s", job.Stage, ReturnStageUpdateRating)
	}
}

func TestReturnDoesNotQueuePermanentFailure(t *testing.T) {
	gateway, library, _, _, queue := newTestGatewayWithQueue()
	date := time.Date(2021, 10, 11, 0, 0, 0, 0, time.UTC)
	details, err := gateway.Rent(context.Background(), "alice", library.library.Id, library.book.Id, date)
	if err != nil {
		t.Fatal(err)
	}
	library.returnErr = domain.ErrBookNotFound
	err = gateway.Return(context.Background(), details.Reservation.Id, "alice", domain.ConditionExcellent, date)
	if !errors.Is(err, domain.ErrBookNotFound) || len(queue.jobs) != 0 {
		t.Fatalf("permanent failure = %v, queued jobs=%d", err, len(queue.jobs))
	}
}

func TestReturnReportsPublishFailure(t *testing.T) {
	gateway, library, _, _, queue := newTestGatewayWithQueue()
	date := time.Date(2021, 10, 11, 0, 0, 0, 0, time.UTC)
	details, err := gateway.Rent(context.Background(), "alice", library.library.Id, library.book.Id, date)
	if err != nil {
		t.Fatal(err)
	}
	library.returnErr = &DependencyError{Service: "library", Cause: errors.New("timeout")}
	queue.err = errors.New("Redis unavailable")
	err = gateway.Return(context.Background(), details.Reservation.Id, "alice", domain.ConditionExcellent, date)
	if !errors.Is(err, queue.err) || len(queue.jobs) != 0 {
		t.Fatalf("publish failure = %v", err)
	}
}
