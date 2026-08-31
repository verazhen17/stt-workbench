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
	samplesFilesystem := os.DirFS(settings.SamplesRoot)
	streamCatalog := domain.NewFilesystemStreamCatalog(samplesFilesystem)
	presetCatalog := domain.NewFilesystemPresetCatalog(samplesFilesystem)
	resultCatalog := domain.NewFilesystemResultCatalog(samplesFilesystem)
	selectableResults := domain.NewSelectableResultCatalog(presetCatalog, resultCatalog)
	vodCatalog, err := domain.NewFilesystemVODCatalog(
		samplesFilesystem,
		settings.SamplesRoot,
		settings.VODURLPrefix,
		domain.NewFFprobeDurationProber(settings.FFprobePath),
	)
	if err != nil {
		return err
	}
	engine := router.NewRouter(router.Dependencies{
		Streams: streamCatalog,
		Presets: presetCatalog,
		VODs:    vodCatalog,
		Results: selectableResults,
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
