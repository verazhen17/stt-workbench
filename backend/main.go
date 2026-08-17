package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/router"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("backend stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	settings := router.LoadConfig()
	streamCatalog := domain.NewFilesystemStreamCatalog(os.DirFS(settings.VODRoot))
	engine := router.NewRouter(router.Dependencies{
		Streams: streamCatalog,
		Logger:  logger,
	})

	listener, err := net.Listen("tcp", settings.HTTPAddr)
	if err != nil {
		return err
	}
	logger.Info("backend listening", "address", listener.Addr().String())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return router.NewServer(engine, logger).Serve(ctx, listener)
}
