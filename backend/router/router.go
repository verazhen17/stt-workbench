package router

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/models"
	"github.com/gin-gonic/gin"
)

const shutdownTimeout = 5 * time.Second

type Dependencies struct {
	Streams domain.StreamCatalog
	Presets domain.PresetCatalog
	VODs    domain.VODCatalog
	Results domain.SelectableResultLister
	Logger  *slog.Logger
}

type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

type streamHandler struct {
	streams domain.StreamCatalog
	presets domain.PresetCatalog
	vods    domain.VODCatalog
	results domain.SelectableResultLister
	logger  *slog.Logger
}

type streamsResponse struct {
	Streams []models.Stream `json:"streams"`
}

type presetsResponse struct {
	Presets []models.Preset `json:"presets"`
}

func NewRouter(dependencies Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(requestLogger(dependencies.Logger), recovery(dependencies.Logger))

	handler := streamHandler{
		streams: dependencies.Streams,
		presets: dependencies.Presets,
		vods:    dependencies.VODs,
		results: dependencies.Results,
		logger:  dependencies.Logger,
	}
	router.GET("/api/presets", handler.listPresets)
	router.GET("/api/streams", handler.list)
	router.GET("/api/streams/:stream_id", handler.detail)
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
	requestContext := context.Request.Context()
	var streams []models.Stream
	presetID, hasPresetID := context.GetQuery("preset_id")
	if hasPresetID {
		if !domain.IsValidUUID(presetID) {
			writeError(context, http.StatusBadRequest, "invalid_preset_id", "preset_id must be a UUID.", nil)
			return
		}
		preset, err := handler.presets.Get(requestContext, presetID)
		if err != nil {
			handler.writeCatalogError(context, err, "preset")
			return
		}
		allStreams, err := handler.streams.List(requestContext)
		if err != nil {
			handler.writeCatalogError(context, err, "list streams")
			return
		}
		streams = make([]models.Stream, 0, len(allStreams))
		for _, stream := range allStreams {
			if containsStreamID(preset.StreamIDs, stream.StreamID) {
				streams = append(streams, stream)
			}
		}
	} else {
		var err error
		streams, err = handler.streams.List(requestContext)
		if err != nil {
			handler.writeCatalogError(context, err, "list streams")
			return
		}
	}
	if streams == nil {
		streams = []models.Stream{}
	}
	context.JSON(http.StatusOK, streamsResponse{Streams: streams})
}

func (handler streamHandler) listPresets(context *gin.Context) {
	presets, err := handler.presets.List(context.Request.Context())
	if err != nil {
		handler.writeCatalogError(context, err, "list presets")
		return
	}
	if presets == nil {
		presets = []models.Preset{}
	}
	context.JSON(http.StatusOK, presetsResponse{Presets: presets})
}

func (handler streamHandler) detail(context *gin.Context) {
	streamID := context.Param("stream_id")
	vods, err := handler.vods.List(context.Request.Context(), streamID)
	if err != nil {
		handler.writeCatalogError(context, err, "list VODs")
		return
	}
	if vods == nil {
		vods = []models.VODSegment{}
	}
	for index := range vods {
		results, err := handler.results.List(context.Request.Context(), streamID, vods[index].VODID)
		if err != nil {
			handler.writeCatalogError(context, err, "list STT results")
			return
		}
		if results == nil {
			results = []models.STTResultSummary{}
		}
		vods[index].STTResults = results
	}
	context.JSON(http.StatusOK, models.StreamDetail{StreamID: streamID, VODs: vods})
}

func (handler streamHandler) writeCatalogError(context *gin.Context, err error, operation string) {
	handler.logger.Error(operation, "error", err)
	switch {
	case errors.Is(err, domain.ErrPresetNotFound), errors.Is(err, domain.ErrSTTResultNotFound), errors.Is(err, domain.ErrStreamNotFound):
		code, message := "not_found", "The requested resource was not found."
		if errors.Is(err, domain.ErrPresetNotFound) {
			code, message = "preset_not_found", "The requested preset was not found."
		} else if errors.Is(err, domain.ErrSTTResultNotFound) {
			code, message = "stt_result_not_found", "The requested STT result was not found."
		} else if errors.Is(err, domain.ErrStreamNotFound) {
			code, message = "stream_not_found", "The requested stream was not found."
		}
		writeError(context, http.StatusNotFound, code, message, nil)
	case errors.Is(err, domain.ErrVODIngestionFailed):
		writeError(context, http.StatusUnprocessableEntity, "vod_ingestion_failed", "The VOD could not be ingested.", nil)
	case errors.Is(err, domain.ErrPresetIndexInconsistent):
		writeError(context, http.StatusConflict, "preset_index_inconsistent", "Preset completion index is inconsistent with its canonical results.", nil)
	default:
		writeError(context, http.StatusInternalServerError, "internal_error", "An internal error occurred.", nil)
	}
}

func containsStreamID(streamIDs []string, target string) bool {
	for _, streamID := range streamIDs {
		if streamID == target {
			return true
		}
	}
	return false
}
