package router

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path"
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
	GoldenManager  domain.GoldenManager
	Logger         *slog.Logger
}

type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

type streamHandler struct {
	streams       domain.StreamCatalog
	presets       domain.PresetCatalog
	vods          domain.VODCatalog
	results       domain.SelectableResultLister
	provider      domain.SelectableResultProvider
	golden        domain.GoldenReader
	goldenManager domain.GoldenManager
	logger        *slog.Logger
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
		streams:       dependencies.Streams,
		presets:       dependencies.Presets,
		vods:          dependencies.VODs,
		results:       dependencies.Results,
		provider:      dependencies.ResultProvider,
		golden:        dependencies.Golden,
		goldenManager: dependencies.GoldenManager,
		logger:        dependencies.Logger,
	}
	router.GET("/api/presets", handler.listPresets)
	router.GET("/api/streams", handler.list)
	router.GET("/api/streams/:stream_id", handler.detail)
	router.GET("/api/streams/:stream_id/align", handler.align)
	router.PUT("/api/streams/:stream_id/golden", handler.saveGolden)
	router.POST("/api/export", handler.export)
	return router
}

type exportRequest struct {
	StreamIDs   []string `json:"stream_ids"`
	DataSources []string `json:"data_sources"`
}

func (handler streamHandler) export(context *gin.Context) {
	var request exportRequest
	if err := context.ShouldBindJSON(&request); err != nil || len(request.StreamIDs) == 0 || len(request.DataSources) == 0 {
		writeError(context, http.StatusBadRequest, "invalid_export_request", "stream_ids and data_sources are required.", nil)
		return
	}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	manifest := map[string]any{"stream_ids": request.StreamIDs, "data_sources": request.DataSources}
	manifestStreams := make([]map[string]any, 0, len(request.StreamIDs))
	for _, streamID := range request.StreamIDs {
		vods, err := handler.vods.List(context.Request.Context(), streamID)
		if err != nil {
			writer.Close()
			handler.writeCatalogError(context, err, "export VODs")
			return
		}
		streamManifest := map[string]any{"stream_id": streamID, "vods": []string{}}
		vodNames := make([]string, 0, len(vods))
		for _, vod := range vods {
			vodNames = append(vodNames, vod.VODID)
			usedNames := map[string]int{}
			for _, source := range request.DataSources {
				var data any
				var name string
				if source == "golden" {
					golden, goldenErr := handler.golden.Get(context.Request.Context(), streamID, vod.VODID)
					if errors.Is(goldenErr, domain.ErrGoldenNotFound) {
						continue
					}
					if goldenErr != nil {
						writer.Close()
						handler.writeCatalogError(context, goldenErr, "export Golden")
						return
					}
					data, name = golden, "golden.json"
				} else {
					result, preset, resultErr := handler.provider.Get(context.Request.Context(), streamID, vod.VODID, source)
					if errors.Is(resultErr, domain.ErrSTTResultNotFound) || errors.Is(resultErr, domain.ErrPresetIndexInconsistent) {
						continue
					}
					if resultErr != nil {
						writer.Close()
						handler.writeCatalogError(context, resultErr, "export STT result")
						return
					}
					data = result
					presetName := preset.Name
					if presetName == "" {
						presetName = preset.Model.Name
					}
					name = safeExportFilename(presetName) + ".json"
				}
				baseName := strings.TrimSuffix(name, ".json")
				if count := usedNames[baseName]; count > 0 {
					name = fmt.Sprintf("%s_%d.json", baseName, count+1)
				}
				usedNames[baseName]++
				payload, marshalErr := json.MarshalIndent(data, "", "  ")
				if marshalErr != nil {
					writer.Close()
					context.JSON(http.StatusInternalServerError, gin.H{"error": marshalErr.Error()})
					return
				}
				file, createErr := writer.Create(path.Join(streamID, vod.VODID, name))
				if createErr != nil {
					writer.Close()
					context.JSON(http.StatusInternalServerError, gin.H{"error": createErr.Error()})
					return
				}
				if _, writeErr := file.Write(payload); writeErr != nil {
					writer.Close()
					context.JSON(http.StatusInternalServerError, gin.H{"error": writeErr.Error()})
					return
				}
			}
		}
		streamManifest["vods"] = vodNames
		manifestStreams = append(manifestStreams, streamManifest)
	}
	manifest["streams"] = manifestStreams
	manifestPayload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		writer.Close()
		context.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	manifestFile, err := writer.Create("manifest.json")
	if err != nil {
		writer.Close()
		context.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if _, err = manifestFile.Write(manifestPayload); err != nil {
		writer.Close()
		context.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err = writer.Close(); err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.Data(http.StatusOK, "application/zip", archive.Bytes())
}

func safeExportFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unnamed-preset"
	}
	var builder strings.Builder
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			builder.WriteRune(character)
		} else {
			builder.WriteRune('_')
		}
	}
	return strings.Trim(builder.String(), "_.")
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
			item.Error = &models.SelectedResultError{Code: "invalid_segment", Message: fmt.Sprintf("STT result contains invalid segments: %v", err)}
		} else {
			validResults = append(validResults, result)
		}
		selected = append(selected, item)
	}

	golden, err := handler.golden.Get(context.Request.Context(), streamID, vodID)
	if errors.Is(err, domain.ErrGoldenNotFound) {
		if len(validResults) > 0 && selected[0].Error == nil {
			golden = models.Golden{StreamID: streamID, VODID: vodID, Segments: make([]models.GoldenSegment, 0, len(validResults[0].Segments))}
			for index, segment := range validResults[0].Segments {
				golden.Segments = append(golden.Segments, models.GoldenSegment{SegmentID: fmt.Sprintf("golden_segment_%03d", index+1), Timestamps: segment.Timestamps, Text: segment.Text})
			}
		} else {
			golden = models.Golden{StreamID: streamID, VODID: vodID}
		}
	} else if err != nil {
		handler.writeCatalogError(context, err, "read Golden")
		return
	}

	rows := []models.AlignmentRow{}
	warnings := []models.AlignmentWarning{}
	if len(validResults) > 0 && (golden.VODID == "" || golden.VODID == vodID) {
		rows, warnings, err = domain.Align(golden, validResults)
		if err != nil {
			handler.writeCatalogError(context, err, "align results")
			return
		}
	}
	context.JSON(http.StatusOK, models.Alignment{StreamID: streamID, VODID: vodID, SelectedResults: selected, Rows: rows, Warnings: warnings})
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

func (handler streamHandler) saveGolden(context *gin.Context) {
	var request models.GoldenRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		writeError(context, http.StatusBadRequest, "invalid_request", "Request body is invalid.", nil)
		return
	}
	if request.VODID == "" {
		writeError(context, http.StatusBadRequest, "invalid_request", "vod_id is required.", nil)
		return
	}

	var (
		golden models.Golden
		err    error
	)
	switch request.Mode {
	case "renew":
		if !domain.IsValidUUID(request.SourcePresetID) {
			writeError(context, http.StatusBadRequest, "invalid_request", "source_preset_id must be a UUID.", nil)
			return
		}
		golden, err = handler.goldenManager.Renew(context.Request.Context(), context.Param("stream_id"), request.VODID, request.SourcePresetID)
	case "edit":
		golden, err = handler.goldenManager.Edit(context.Request.Context(), context.Param("stream_id"), request.VODID, request.Segments)
	default:
		writeError(context, http.StatusBadRequest, "invalid_request", "mode must be renew or edit.", nil)
		return
	}
	if err != nil {
		handler.writeCatalogError(context, err, "save Golden")
		return
	}
	context.JSON(http.StatusOK, golden)
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
		writeError(context, http.StatusUnprocessableEntity, "golden_validation_failed", "Golden segments are invalid.", nil)
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
