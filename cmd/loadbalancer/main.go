package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/anujdhakrey/load-balancer/internal/admin"
	"github.com/anujdhakrey/load-balancer/internal/config"
	"github.com/anujdhakrey/load-balancer/internal/health"
	"github.com/anujdhakrey/load-balancer/internal/pool"
	"github.com/anujdhakrey/load-balancer/internal/proxy"
	"github.com/anujdhakrey/load-balancer/internal/ratelimiter"
	"github.com/anujdhakrey/load-balancer/internal/strategy"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn, error")
	flag.Parse()

	logger := setupLogger(*logLevel)

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("configuration loaded",
		"strategy", cfg.Strategy,
		"backends", len(cfg.Backends),
		"server_port", cfg.Server.Port,
	)

	// Initialize backend pool.
	backendPool := pool.New(logger)
	if err := backendPool.InitFromConfig(cfg.Backends); err != nil {
		logger.Error("failed to initialize backend pool", "error", err)
		os.Exit(1)
	}

	// Select load balancing strategy.
	strat := selectStrategy(cfg.Strategy)
	logger.Info("strategy selected", "name", strat.Name())

	// Initialize rate limiter (optional).
	var limiter *ratelimiter.RateLimiter
	if cfg.RateLimit.Enabled {
		limiter = ratelimiter.New(
			cfg.RateLimit.Rate,
			cfg.RateLimit.BurstSize,
			cfg.RateLimit.PerIP,
			cfg.RateLimit.CleanupSec,
		)
		logger.Info("rate limiter enabled",
			"rate", cfg.RateLimit.Rate,
			"burst", cfg.RateLimit.BurstSize,
			"per_ip", cfg.RateLimit.PerIP,
		)
	}

	// Create the load balancer handler.
	lb := proxy.New(backendPool, strat, limiter, cfg.Server.MaxRetries, logger)

	// Context for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start health checker.
	healthChecker := health.NewChecker(
		backendPool,
		cfg.HealthCheck.Interval,
		cfg.HealthCheck.Timeout,
		cfg.HealthCheck.Path,
		logger,
	)
	go healthChecker.Run(ctx)

	var wg sync.WaitGroup

	// Start the main proxy server.
	proxyAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	proxyServer := &http.Server{
		Addr:         proxyAddr,
		Handler:      lb,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("load balancer listening", "addr", proxyAddr)
		if err := proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("proxy server error", "error", err)
			cancel()
		}
	}()

	// Start the admin server (optional).
	var adminServer *http.Server
	if cfg.Admin.Enabled {
		adminAddr := fmt.Sprintf("%s:%d", cfg.Admin.Host, cfg.Admin.Port)
		adminHandler := admin.NewServer(backendPool, logger)
		adminServer = &http.Server{
			Addr:         adminAddr,
			Handler:      adminHandler.Handler(),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("admin server listening", "addr", adminAddr)
			if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("admin server error", "error", err)
			}
		}()
	}

	// Wait for termination signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("received shutdown signal", "signal", sig)
	case <-ctx.Done():
	}

	// Graceful shutdown with a timeout.
	logger.Info("initiating graceful shutdown...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	cancel() // Stop health checker.

	if err := proxyServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("proxy server shutdown error", "error", err)
	}
	if adminServer != nil {
		if err := adminServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("admin server shutdown error", "error", err)
		}
	}

	wg.Wait()
	logger.Info("shutdown complete")
}

func selectStrategy(name string) strategy.Strategy {
	switch name {
	case "round-robin":
		return strategy.NewRoundRobin()
	case "weighted-round-robin":
		return strategy.NewWeightedRoundRobin()
	case "least-connections":
		return strategy.NewLeastConnections()
	default:
		return strategy.NewRoundRobin()
	}
}

func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	return slog.New(handler)
}
