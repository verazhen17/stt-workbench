package router

import "github.com/gin-gonic/gin"

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func writeError(context *gin.Context, status int, code, message string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	context.AbortWithStatusJSON(status, errorEnvelope{Error: errorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}
