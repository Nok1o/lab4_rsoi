package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
	"libriary_system/shared/log"
)

type Config struct {
	Stream         string
	Group          string
	Workers        int
	RetryDelay     time.Duration
	AttemptTimeout time.Duration
}

type LibraryService interface {
	GetBookByUID(ctx context.Context, libraryUID, bookUID uuid.UUID) (domain.LibraryBook, error)
	ReturnBook(ctx context.Context, libraryUID, bookUID uuid.UUID, condition domain.BookCondition) error
}

type RatingService interface {
	AddStars(ctx context.Context, username string, delta int) (domain.Rating, error)
}

type ReturnQueue struct {
	client    *redis.Client
	config    Config
	libraries LibraryService
	ratings   RatingService
}

func NewReturnQueue(client *redis.Client, config Config, libraries LibraryService, ratings RatingService) *ReturnQueue {
	if config.Stream == "" {
		config.Stream = "gateway:{returns}:jobs"
	}
	if config.Group == "" {
		config.Group = "return-workers"
	}
	if config.Workers <= 0 {
		config.Workers = 2
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 10 * time.Second
	}
	if config.AttemptTimeout <= 0 {
		config.AttemptTimeout = 10 * time.Second
	}
	return &ReturnQueue{client: client, config: config, libraries: libraries, ratings: ratings}
}

// Init starts at 0 so messages published before the group existed are read too.
func (queue *ReturnQueue) Init(ctx context.Context) error {
	err := queue.client.XGroupCreateMkStream(ctx, queue.config.Stream, queue.config.Group, "0").Err()
	if err != nil && !redis.HasErrorPrefix(err, "BUSYGROUP") {
		return fmt.Errorf("create return consumer group: %w", err)
	}
	return nil
}

func (queue *ReturnQueue) Enqueue(ctx context.Context, job usecase.ReturnJob) error {
	payload, err := encodeJob(job)
	if err != nil {
		return err
	}
	if err := queue.client.XAdd(ctx, &redis.XAddArgs{
		Stream: queue.config.Stream,
		Values: map[string]any{"payload": payload},
	}).Err(); err != nil {
		return fmt.Errorf("publish deferred return: %w", err)
	}
	return nil
}

// Workers process one step per delivery. Temporary failures stay pending;
// completed steps are acknowledged together with publication of the next step.
func (queue *ReturnQueue) Run(ctx context.Context) {
	var group sync.WaitGroup
	for range queue.config.Workers {
		group.Go(func() { queue.runWorker(ctx, uuid.NewString()) })
	}
	group.Wait()
}

func (queue *ReturnQueue) runWorker(ctx context.Context, consumer string) {
	cursor := "0-0"
	for ctx.Err() == nil {
		// Retry failed or abandoned deliveries. Do not reclaim a message before
		// its previous attempt's deadline has elapsed.
		messages, next, err := queue.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream: queue.config.Stream, Group: queue.config.Group, Consumer: consumer,
			MinIdle: max(queue.config.RetryDelay, queue.config.AttemptTimeout+time.Second),
			Start:   cursor, Count: 1,
		}).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			queue.redisFailure(ctx, err)
			continue
		}
		cursor = next
		if len(messages) > 0 {
			queue.handle(ctx, messages[0])
			continue
		}
		streams, err := queue.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: queue.config.Group, Consumer: consumer,
			Streams: []string{queue.config.Stream, ">"}, Count: 1, Block: time.Second,
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			queue.redisFailure(ctx, err)
			continue
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				queue.handle(ctx, message)
			}
		}
	}
}

func (queue *ReturnQueue) handle(ctx context.Context, message redis.XMessage) {
	job, err := decodeJob(message.Values["payload"])
	if err != nil {
		log.Error("discard malformed return job", "message_id", message.ID, "error", err)
		queue.finish(ctx, message, nil)
		return
	}
	attemptCtx, cancel := context.WithTimeout(ctx, queue.config.AttemptTimeout)
	nextJob, done, err := queue.processOnce(attemptCtx, job)
	cancel()
	if ctx.Err() != nil {
		// Leave pending on shutdown for another worker.
		return
	}
	if err != nil {
		if usecase.IsRetryable(err) {
			log.Warn("deferred return attempt failed; delivery remains pending",
				"reservation_uid", job.ReservationUID, "stage", job.Stage, "error", err)
			return
		}
		log.Error("discard deferred return after permanent failure",
			"reservation_uid", job.ReservationUID, "stage", job.Stage, "error", err)
		queue.finish(ctx, message, nil)
		return
	}
	if done {
		queue.finish(ctx, message, nil)
		return
	}
	queue.finish(ctx, message, &nextJob)
}

func (queue *ReturnQueue) processOnce(ctx context.Context, job usecase.ReturnJob) (usecase.ReturnJob, bool, error) {
	switch job.Stage {
	case usecase.ReturnStageRestoreBook:
		if job.OriginalCondition == "" {
			book, err := queue.libraries.GetBookByUID(ctx, job.LibraryUID, job.BookUID)
			if err != nil {
				return job, false, err
			}
			job.OriginalCondition = book.Condition
			// Stream entries are immutable. Persist the initial condition before
			// changing the book so a normal retry keeps the same rating input.
			return job, false, nil
		}
		if err := queue.libraries.ReturnBook(ctx, job.LibraryUID, job.BookUID, job.ReturnedCondition); err != nil {
			return job, false, err
		}
		job.Stage = usecase.ReturnStageUpdateRating
		return job, false, nil
	case usecase.ReturnStageUpdateRating:
		_, err := queue.ratings.AddStars(ctx, job.Username, job.RatingDelta())
		return job, err == nil, err
	default:
		return job, false, fmt.Errorf("unknown return stage %q", job.Stage)
	}
}

// Publish the next step and acknowledge/remove the previous delivery in one
// Redis transaction. HTTP calls remain outside this transaction.
func (queue *ReturnQueue) finish(ctx context.Context, message redis.XMessage, nextJob *usecase.ReturnJob) {
	var payload string
	if nextJob != nil {
		var err error
		payload, err = encodeJob(*nextJob)
		if err != nil {
			log.Error("encode next return step", "message_id", message.ID, "error", err)
			return
		}
	}
	_, err := queue.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		if nextJob != nil {
			pipe.XAdd(ctx, &redis.XAddArgs{
				Stream: queue.config.Stream,
				Values: map[string]any{"payload": payload},
			})
		}
		pipe.XAck(ctx, queue.config.Stream, queue.config.Group, message.ID)
		pipe.XDel(ctx, queue.config.Stream, message.ID)
		return nil
	})
	if err != nil {
		log.Error("save return queue state", "message_id", message.ID, "error", err)
	}
}

func (queue *ReturnQueue) redisFailure(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	log.Error("read return stream", "error", err)
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
