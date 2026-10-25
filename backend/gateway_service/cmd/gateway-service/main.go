package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	libraryclient "libriary_system/gateway_service/internal/client/library"
	ratingclient "libriary_system/gateway_service/internal/client/rating"
	reservationclient "libriary_system/gateway_service/internal/client/reservation"
	"libriary_system/gateway_service/internal/config"
	gatewayhttp "libriary_system/gateway_service/internal/delivery/http"
	returnqueue "libriary_system/gateway_service/internal/queue"
	"libriary_system/gateway_service/internal/usecase"
	"libriary_system/pkg"
	"libriary_system/shared/log"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Error("load configuration", "error", err)
		os.Exit(1)
	}
	breakerConfig := pkg.CircuitBreakerConfig{
		FailureThreshold: 3,
		MaxHalfOpen:      1,
		OpenTimeout:      10 * time.Second,
	}
	libraryBreaker := pkg.NewCircuitBreaker(breakerConfig)
	ratingBreaker := pkg.NewCircuitBreaker(breakerConfig)
	reservationBreaker := pkg.NewCircuitBreaker(breakerConfig)

	libraries, err := libraryclient.NewClient(cfg.LibraryServiceURL, nil, libraryBreaker)
	if err != nil {
		log.Error("configure library client", "error", err)
		os.Exit(1)
	}
	ratings, err := ratingclient.NewClient(cfg.RatingServiceURL, nil, ratingBreaker)
	if err != nil {
		log.Error("configure rating client", "error", err)
		os.Exit(1)
	}
	reservations, err := reservationclient.NewClient(cfg.ReservationServiceURL, nil, reservationBreaker)
	if err != nil {
		log.Error("configure reservation client", "error", err)
		os.Exit(1)
	}

	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Error("configure Redis", "error", err)
		os.Exit(1)
	}
	redisOptions.ContextTimeoutEnabled = true
	redisClient := redis.NewClient(redisOptions)
	defer redisClient.Close()
	returnQueue := returnqueue.NewReturnQueue(redisClient, returnqueue.Config{
		Workers:        3,
		RetryDelay:     10 * time.Second,
		AttemptTimeout: 10 * time.Second,
	}, libraries, ratings)
	initCtx, cancelInit := context.WithTimeout(context.Background(), 5*time.Second)
	err = returnQueue.Init(initCtx)
	cancelInit()
	if err != nil {
		log.Error("initialize return queue", "error", err)
		os.Exit(1)
	}
	queueCtx, cancelQueue := context.WithCancel(context.Background())
	defer cancelQueue()
	queueDone := make(chan struct{})
	go func() {
		defer close(queueDone)
		returnQueue.Run(queueCtx)
	}()

	gateway := usecase.NewGatewayUseCase(libraries, ratings, reservations, returnQueue)
	handler := gatewayhttp.NewHandler(gateway)
	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           gatewayhttp.NewRouter(handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Info("gateway service started", "address", cfg.HTTPAddress)
		serverErrors <- server.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case signal := <-signals:
		log.Info("shutdown signal received", "signal", signal.String())
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			cancelQueue()
			<-queueDone
			log.Error("HTTP server stopped", "error", err)
			os.Exit(1)
		}
		return
	}
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		cancelQueue()
		<-queueDone
		log.Error("graceful shutdown", "error", err)
		os.Exit(1)
	}
	cancelQueue()
	<-queueDone
	log.Info("gateway service stopped")
}
