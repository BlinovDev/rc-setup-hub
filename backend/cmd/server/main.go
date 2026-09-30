package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/admin"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/setups"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
	"github.com/go-chi/chi/v5"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 && attr.Key == slog.TimeKey {
				attr.Value = slog.TimeValue(attr.Value.Time().UTC())
			}
			return attr
		},
	}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	authConfig, err := config.LoadAuth()
	if err != nil {
		logger.Error("authentication configuration failed", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	pool, err := database.Open(connectCtx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		logger.Error("database startup failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	router := web.NewRouter(logger, pool.Ping)
	userService := users.NewService(users.NewRepository(pool))
	authHandler := auth.NewHandler(authConfig, auth.NewGoogle(authConfig), userService)
	catalog := chassis.NewService(chassis.NewRepository(pool))
	catalogHandler, err := chassis.NewHandler(catalog)
	if err != nil {
		logger.Error("catalog initialization failed")
		os.Exit(1)
	}
	friendshipRepository := friendships.NewRepository(pool)
	friendshipHandler := friendships.NewHandler(friendships.NewService(friendshipRepository))
	userHandler := users.NewHandler(userService)
	setupService := setups.NewService(setups.NewRepository(pool), catalog, friendshipRepository)
	setupHandler := setups.NewHandler(setupService)
	authHandler.Register(router, catalogHandler.RegisterAPI, setupHandler.RegisterAPI, friendshipHandler.RegisterAPI, func(r chi.Router) { userHandler.RegisterAPI(r, auth.CurrentUser) })
	adminHandler, err := admin.New(authConfig)
	if err != nil {
		logger.Error("admin initialization failed")
		os.Exit(1)
	}
	adminHandler.Register(router, authHandler.RequireUser, catalogHandler.RegisterAdmin)
	server := &http.Server{
		Addr: cfg.HTTPAddr, Handler: router,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	logger.Info("server starting", "http_addr", cfg.HTTPAddr)
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err)
			_ = server.Close()
			os.Exit(1)
		}
		if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
		logger.Info("server stopped")
	}
}
