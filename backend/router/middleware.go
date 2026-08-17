package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(context *gin.Context) {
		startedAt := time.Now()
		context.Next()
		logger.Info("HTTP request",
			"method", context.Request.Method,
			"path", context.Request.URL.Path,
			"status", context.Writer.Status(),
			"duration", time.Since(startedAt),
		)
	}
}

func recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(context *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic handling HTTP request", "panic", recovered)
				writeError(context, http.StatusInternalServerError, "internal_error", "An internal error occurred.", nil)
			}
		}()
		context.Next()
	}
}
