package router

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/gin-gonic/gin"
)

const shutdownTimeout = 5 * time.Second

type Dependencies struct {
	Streams domain.StreamCatalog
	Logger  *slog.Logger
}

type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

type streamHandler struct {
	catalog domain.StreamCatalog
	logger  *slog.Logger
}

type streamsResponse struct {
	Streams []domain.Stream `json:"streams"`
}

func NewRouter(dependencies Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(requestLogger(dependencies.Logger), recovery(dependencies.Logger))

	handler := streamHandler{
		catalog: dependencies.Streams,
		logger:  dependencies.Logger,
	}
	router.GET("/api/streams", handler.list)
	return router
}

func NewServer(handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
			ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		},
		logger: logger,
	}
}

func (server *Server) Serve(ctx context.Context, listener net.Listener) error {
	serveContext, stop := context.WithCancel(ctx)
	defer stop()

	shutdownComplete := make(chan struct{})
	go func() {
		defer close(shutdownComplete)
		<-serveContext.Done()

		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.httpServer.Shutdown(shutdownContext); err != nil {
			server.logger.Error("graceful shutdown failed", "error", err)
		}
	}()

	err := server.httpServer.Serve(listener)
	stop()
	<-shutdownComplete
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (handler streamHandler) list(context *gin.Context) {
	streams, err := handler.catalog.List(context.Request.Context())
	if err != nil {
		handler.logger.Error("list streams", "error", err)
		writeError(context, http.StatusInternalServerError, "internal_error", "An internal error occurred.", nil)
		return
	}
	if streams == nil {
		streams = []domain.Stream{}
	}
	context.JSON(http.StatusOK, streamsResponse{Streams: streams})
}
