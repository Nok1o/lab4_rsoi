package queue

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
)

type fakeLibraryService struct {
	calls    int
	getCalls int
	err      error
}

func (service *fakeLibraryService) GetBookByUID(_ context.Context, _, _ uuid.UUID) (domain.LibraryBook, error) {
	service.getCalls++
	return domain.LibraryBook{Book: domain.Book{Condition: domain.ConditionExcellent}}, nil
}

func (service *fakeLibraryService) ReturnBook(_ context.Context, _, _ uuid.UUID, _ domain.BookCondition) error {
	service.calls++
	return service.err
}

type fakeRatingService struct {
	mu       sync.Mutex
	failures map[string]int
	calls    chan string
	err      error
}

func (service *fakeRatingService) AddStars(_ context.Context, username string, _ int) (domain.Rating, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.calls != nil {
		service.calls <- username
	}
	if service.err != nil {
		return domain.Rating{}, service.err
	}
	if service.failures[username] > 0 {
		service.failures[username]--
		return domain.Rating{}, &usecase.DependencyError{Service: "rating", Cause: errors.New("unavailable")}
	}
	return domain.Rating{Username: username, StarsCount: 76}, nil
}

func testJob(username string) usecase.ReturnJob {
	return usecase.ReturnJob{ReservationUID: uuid.New(), LibraryUID: uuid.New(), BookUID: uuid.New(),
		Username: username, OriginalCondition: domain.ConditionExcellent,
		ReturnedCondition: domain.ConditionExcellent, Stage: usecase.ReturnStageUpdateRating}
}

func TestProcessOnceCheckpointsBeforeChangingBook(t *testing.T) {
	libraries := &fakeLibraryService{}
	ratings := &fakeRatingService{calls: make(chan string, 1)}
	queue := NewReturnQueue(nil, Config{}, libraries, ratings)
	job := testJob("alice")
	job.Stage = usecase.ReturnStageRestoreBook
	job.OriginalCondition = ""
	job.ReturnedCondition = domain.ConditionBad

	next, done, err := queue.processOnce(context.Background(), job)
	if err != nil || done || next.Stage != usecase.ReturnStageRestoreBook || next.OriginalCondition != domain.ConditionExcellent {
		t.Fatalf("initial condition checkpoint = (%+v, %t, %v)", next, done, err)
	}
	if libraries.getCalls != 1 || libraries.calls != 0 || len(ratings.calls) != 0 {
		t.Fatal("initial condition must be checkpointed before modifying the book")
	}
	next, done, err = queue.processOnce(context.Background(), next)
	if err != nil || done || next.Stage != usecase.ReturnStageUpdateRating || next.RatingDelta() != -10 {
		t.Fatalf("stage transition = (%+v, %t, %v)", next, done, err)
	}
	if libraries.getCalls != 1 || libraries.calls != 1 || len(ratings.calls) != 0 {
		t.Fatal("restore must checkpoint its completion before calling rating")
	}
}

func TestProcessOnceKeepsOriginalConditionAfterFailedRestore(t *testing.T) {
	libraries := &fakeLibraryService{err: &usecase.DependencyError{Service: "library", Cause: errors.New("timeout")}}
	queue := NewReturnQueue(nil, Config{}, libraries, &fakeRatingService{})
	job := testJob("alice")
	job.Stage = usecase.ReturnStageRestoreBook
	job.ReturnedCondition = domain.ConditionBad
	next, _, err := queue.processOnce(context.Background(), job)
	if err == nil || next.OriginalCondition != domain.ConditionExcellent || next.Stage != usecase.ReturnStageRestoreBook {
		t.Fatalf("failed restore = (%+v, %v)", next, err)
	}
	libraries.err = nil
	next, _, err = queue.processOnce(context.Background(), next)
	if err != nil || next.RatingDelta() != -10 || libraries.getCalls != 0 || libraries.calls != 2 {
		t.Fatalf("retry = (%+v, %v), getCalls=%d, returnCalls=%d", next, err, libraries.getCalls, libraries.calls)
	}
}

func integrationQueue(t *testing.T, ratings *fakeRatingService) (*ReturnQueue, *redis.Client) {
	t.Helper()
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		t.Skip("set REDIS_TEST_URL to run Redis integration tests")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	opts.ContextTimeoutEnabled = true
	client := redis.NewClient(opts)
	queue := NewReturnQueue(client, Config{Stream: "test:{" + uuid.NewString() + "}:returns", Workers: 1,
		RetryDelay: 50 * time.Millisecond, AttemptTimeout: 100 * time.Millisecond}, &fakeLibraryService{}, ratings)
	if err := queue.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.Del(context.Background(), queue.config.Stream)
		client.Close()
	})
	return queue, client
}

func runQueue(t *testing.T, queue *ReturnQueue) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); queue.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("queue did not shut down")
		}
	})
}

func eventually(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if check() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("condition did not become true")
		case <-ticker.C:
		}
	}
}

func TestRedisRetryDoesNotBlockFollowingJobs(t *testing.T) {
	ratings := &fakeRatingService{failures: map[string]int{"alice": 1}, calls: make(chan string, 10)}
	queue, client := integrationQueue(t, ratings)
	ctx := context.Background()
	for _, username := range []string{"alice", "bob"} {
		if err := queue.Enqueue(ctx, testJob(username)); err != nil {
			t.Fatal(err)
		}
	}
	runQueue(t, queue)
	for _, want := range []string{"alice", "bob", "alice"} {
		select {
		case got := <-ratings.calls:
			if got != want {
				t.Fatalf("call = %s, want %s", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("missing rating call")
		}
	}
	eventually(t, func() bool {
		return client.XLen(ctx, queue.config.Stream).Val() == 0
	})
}

func TestRedisRetrySurvivesQueueRecreation(t *testing.T) {
	ratings := &fakeRatingService{failures: map[string]int{"alice": 1}, calls: make(chan string, 10)}
	queue, client := integrationQueue(t, ratings)
	ctx := context.Background()
	if err := queue.Enqueue(ctx, testJob("alice")); err != nil {
		t.Fatal(err)
	}
	streams, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: queue.config.Group, Consumer: "old-worker",
		Streams: []string{queue.config.Stream, ">"}, Count: 1}).Result()
	if err != nil {
		t.Fatal(err)
	}
	queue.handle(ctx, streams[0].Messages[0])
	pending, err := client.XPending(ctx, queue.config.Stream, queue.config.Group).Result()
	if err != nil || pending.Count != 1 || client.XLen(ctx, queue.config.Stream).Val() != 1 {
		t.Fatalf("failed delivery must remain pending: (%+v, %v)", pending, err)
	}
	// A fresh queue object has no state from the old process. Redis holds it all.
	fresh := NewReturnQueue(client, queue.config, &fakeLibraryService{}, ratings)
	if err := fresh.Init(ctx); err != nil {
		t.Fatal(err)
	}
	runQueue(t, fresh)
	eventually(t, func() bool { return len(ratings.calls) == 2 && client.XLen(ctx, queue.config.Stream).Val() == 0 })
}

func TestRedisReclaimsCrashedWorkerDelivery(t *testing.T) {
	ratings := &fakeRatingService{calls: make(chan string, 10)}
	queue, client := integrationQueue(t, ratings)
	ctx := context.Background()
	job := testJob("alice")
	job.Stage = usecase.ReturnStageRestoreBook
	job.ReturnedCondition = domain.ConditionBad
	if err := queue.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	streams, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: queue.config.Group, Consumer: "crashed",
		Streams: []string{queue.config.Stream, ">"}, Count: 1}).Result()
	if err != nil {
		t.Fatal(err)
	}
	id := streams[0].Messages[0].ID
	if err := client.Do(ctx, "XCLAIM", queue.config.Stream, queue.config.Group, "crashed", 0, id, "IDLE", 60000).Err(); err != nil {
		t.Fatal(err)
	}
	runQueue(t, queue)
	eventually(t, func() bool { return len(ratings.calls) == 1 && client.XLen(ctx, queue.config.Stream).Val() == 0 })
	if pending := client.XPending(ctx, queue.config.Stream, queue.config.Group).Val(); pending.Count != 0 {
		t.Fatalf("pending = %+v", pending)
	}
}

func TestRedisPermanentErrorsAndMalformedMessagesAreAcknowledged(t *testing.T) {
	ratings := &fakeRatingService{err: domain.ErrRatingNotFound, calls: make(chan string, 10)}
	queue, client := integrationQueue(t, ratings)
	ctx := context.Background()
	if err := queue.Enqueue(ctx, testJob("alice")); err != nil {
		t.Fatal(err)
	}
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: queue.config.Stream, Values: map[string]any{"payload": "bad-json"}}).Err(); err != nil {
		t.Fatal(err)
	}
	runQueue(t, queue)
	eventually(t, func() bool { return client.XLen(ctx, queue.config.Stream).Val() == 0 })
	pending, err := client.XPending(ctx, queue.config.Stream, queue.config.Group).Result()
	if err != nil || pending.Count != 0 || len(ratings.calls) != 1 {
		t.Fatalf("permanent failures should be acknowledged: pending=%+v, calls=%d, err=%v", pending, len(ratings.calls), err)
	}
	keys, err := client.Keys(ctx, queue.config.Stream+"*").Result()
	if err != nil || len(keys) != 1 || keys[0] != queue.config.Stream {
		t.Fatalf("queue should only use one stream: keys=%v, err=%v", keys, err)
	}
}

func TestRedisStageTransitionSurvivesQueueRecreation(t *testing.T) {
	ratings := &fakeRatingService{calls: make(chan string, 10)}
	queue, client := integrationQueue(t, ratings)
	ctx := context.Background()
	job := testJob("alice")
	job.Stage = usecase.ReturnStageRestoreBook
	job.ReturnedCondition = domain.ConditionBad
	if err := queue.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	streams, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: queue.config.Group, Consumer: "old-worker",
		Streams: []string{queue.config.Stream, ">"}, Count: 1}).Result()
	if err != nil {
		t.Fatal(err)
	}
	oldID := streams[0].Messages[0].ID
	queue.handle(ctx, streams[0].Messages[0])

	messages, err := client.XRange(ctx, queue.config.Stream, "-", "+").Result()
	if err != nil || len(messages) != 1 {
		t.Fatalf("stage checkpoint: messages=%v, err=%v", messages, err)
	}
	saved, err := decodeJob(messages[0].Values["payload"])
	if err != nil || saved.Stage != usecase.ReturnStageUpdateRating || saved.RatingDelta() != -10 || messages[0].ID == oldID {
		t.Fatalf("next step = (%+v, %v)", saved, err)
	}
	pending, err := client.XPending(ctx, queue.config.Stream, queue.config.Group).Result()
	if err != nil || pending.Count != 0 {
		t.Fatalf("previous step was not acknowledged: (%+v, %v)", pending, err)
	}

	// A new process must resume at UPDATE_RATING without returning the book.
	fresh := NewReturnQueue(client, queue.config, &fakeLibraryService{err: errors.New("unexpected book restore")}, ratings)
	runQueue(t, fresh)
	eventually(t, func() bool { return len(ratings.calls) == 1 && client.XLen(ctx, queue.config.Stream).Val() == 0 })
}

func TestRedisPersistsOriginalConditionBeforeBookRestore(t *testing.T) {
	queue, client := integrationQueue(t, &fakeRatingService{})
	ctx := context.Background()
	job := testJob("alice")
	job.Stage = usecase.ReturnStageRestoreBook
	job.OriginalCondition = ""
	job.ReturnedCondition = domain.ConditionBad
	if err := queue.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	streams, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: queue.config.Group, Consumer: "worker",
		Streams: []string{queue.config.Stream, ">"}, Count: 1}).Result()
	if err != nil {
		t.Fatal(err)
	}
	queue.handle(ctx, streams[0].Messages[0])
	messages, err := client.XRange(ctx, queue.config.Stream, "-", "+").Result()
	if err != nil || len(messages) != 1 {
		t.Fatalf("checkpoint = (%v, %v)", messages, err)
	}
	saved, err := decodeJob(messages[0].Values["payload"])
	if err != nil || saved.Stage != usecase.ReturnStageRestoreBook || saved.OriginalCondition != domain.ConditionExcellent {
		t.Fatalf("saved initial condition = (%+v, %v)", saved, err)
	}
	libraries := queue.libraries.(*fakeLibraryService)
	if libraries.calls != 0 {
		t.Fatal("book changed before its original condition was persisted")
	}
}
