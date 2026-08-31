package router

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/models"
	"github.com/gin-gonic/gin"
)

const shutdownTimeout = 5 * time.Second

type Dependencies struct {
	Streams        domain.StreamCatalog
	Presets        domain.PresetCatalog
	VODs           domain.VODCatalog
	Results        domain.SelectableResultLister
	ResultProvider domain.SelectableResultProvider
	Golden         domain.GoldenReader
	Logger         *slog.Logger
}

type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

type streamHandler struct {
	streams  domain.StreamCatalog
	presets  domain.PresetCatalog
	vods     domain.VODCatalog
	results  domain.SelectableResultLister
	provider domain.SelectableResultProvider
	golden   domain.GoldenReader
	logger   *slog.Logger
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
		streams:  dependencies.Streams,
		presets:  dependencies.Presets,
		vods:     dependencies.VODs,
		results:  dependencies.Results,
		provider: dependencies.ResultProvider,
		golden:   dependencies.Golden,
		logger:   dependencies.Logger,
	}
	router.GET("/api/presets", handler.listPresets)
	router.GET("/api/streams", handler.list)
	router.GET("/api/streams/:stream_id", handler.detail)
	router.GET("/api/streams/:stream_id/align", handler.align)
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

func (handler streamHandler) align(context *gin.Context) {
	streamID := context.Param("stream_id")
	vodID, hasVODID := context.GetQuery("vod_id")
	if !hasVODID || vodID == "" {
		writeError(context, http.StatusBadRequest, "invalid_request", "vod_id is required.", nil)
		return
	}
	presetIDs, ok := parsePresetIDs(context.Query("preset_ids"))
	if !ok {
		writeError(context, http.StatusBadRequest, "invalid_preset_ids", "preset_ids must contain one or two different UUIDs.", nil)
		return
	}

	vods, err := handler.vods.List(context.Request.Context(), streamID)
	if err != nil {
		handler.writeCatalogError(context, err, "find VOD")
		return
	}
	vodFound := false
	for _, vod := range vods {
		if vod.VODID == vodID {
			vodFound = true
			break
		}
	}
	if !vodFound {
		writeError(context, http.StatusNotFound, "vod_not_found", "The requested VOD was not found.", nil)
		return
	}

	selected := make([]models.SelectedResult, 0, len(presetIDs))
	validResults := make([]models.STTResult, 0, len(presetIDs))
	for _, presetID := range presetIDs {
		result, preset, err := handler.provider.Get(context.Request.Context(), streamID, vodID, presetID)
		if err != nil {
			handler.writeCatalogError(context, err, "select STT result")
			return
		}
		item := models.SelectedResult{PresetID: preset.PresetID, Model: preset.Model, CreatedAt: result.CreatedAt}
		if err := domain.ValidateSTTSegments(result.Segments); err != nil {
			item.Error = &models.SelectedResultError{Code: "invalid_segment", Message: "STT result contains invalid segments."}
		} else {
			validResults = append(validResults, result)
		}
		selected = append(selected, item)
	}

	golden, err := handler.golden.Get(context.Request.Context(), streamID, vodID)
	if errors.Is(err, domain.ErrGoldenNotFound) {
		if len(validResults) > 0 && selected[0].Error == nil {
			golden = models.Golden{StreamID: streamID, VODID: vodID, Segments: make([]models.GoldenSegment, 0, len(validResults[0].Segments))}
			for _, segment := range validResults[0].Segments {
				golden.Segments = append(golden.Segments, models.GoldenSegment{StartMS: segment.StartMS, EndMS: segment.EndMS, Text: segment.Text})
			}
		} else {
			golden = models.Golden{StreamID: streamID, VODID: vodID}
		}
	} else if err != nil {
		handler.writeCatalogError(context, err, "read Golden")
		return
	}

	rows := []models.AlignmentRow{}
	if len(validResults) > 0 && (golden.VODID == "" || golden.VODID == vodID) {
		rows, err = domain.Align(golden, validResults)
		if err != nil {
			handler.writeCatalogError(context, err, "align results")
			return
		}
	}
	context.JSON(http.StatusOK, models.Alignment{StreamID: streamID, VODID: vodID, SelectedResults: selected, Rows: rows})
}

func parsePresetIDs(value string) ([]string, bool) {
	if value == "" {
		return nil, false
	}
	parts := strings.Split(value, ",")
	if len(parts) < 1 || len(parts) > 2 {
		return nil, false
	}
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		if !domain.IsValidUUID(part) || containsStreamID(ids, part) {
			return nil, false
		}
		ids = append(ids, part)
	}
	return ids, true
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
	case errors.Is(err, domain.ErrGoldenNotFound):
		writeError(context, http.StatusNotFound, "golden_not_found", "The requested Golden was not found.", nil)
	case errors.Is(err, domain.ErrGoldenInvalid):
		writeError(context, http.StatusInternalServerError, "internal_error", "An internal error occurred.", nil)
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
