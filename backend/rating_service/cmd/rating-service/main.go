package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"libriary_system/rating_service/internal/config"
	ratinghttp "libriary_system/rating_service/internal/delivery/http"
	"libriary_system/rating_service/internal/repository/postgres"
	"libriary_system/rating_service/internal/usecase"
	"libriary_system/shared/log"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Error("load configuration", "error", err)
		os.Exit(1)
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelStartup()
	pool, err := pgxpool.New(startupCtx, cfg.DatabaseURL)
	if err != nil {
		log.Error("create database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(startupCtx); err != nil {
		log.Error("connect to database", "error", err)
		os.Exit(1)
	}

	repository := postgres.NewRatingPGRepo(pool)
	ratingUseCase := usecase.NewRatingUseCase(repository)
	handler := ratinghttp.NewHandler(ratingUseCase)
	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           ratinghttp.NewRouter(handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Info("rating service started", "address", cfg.HTTPAddress)
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
			log.Error("HTTP server stopped", "error", err)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown", "error", err)
		os.Exit(1)
	}
	log.Info("rating service stopped")
}
