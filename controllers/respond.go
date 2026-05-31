package controllers

import (
	"backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// respondOK 返回成功响应 (HTTP 200)
func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      data,
		OriginUrl: c.Request.URL.Path,
	})
}

// respondErr 返回错误响应，HTTP 状态码与业务码一致
func respondErr(c *gin.Context, httpStatus int, bizCode int, msg string) {
	c.JSON(httpStatus, models.Response{
		Code:      bizCode,
		Message:   msg,
		OriginUrl: c.Request.URL.Path,
	})
}

// respondBadRequest 返回 400 错误
func respondBadRequest(c *gin.Context, msg string) {
	respondErr(c, http.StatusBadRequest, http.StatusBadRequest, msg)
}

// respondNotFound 返回 404 错误
func respondNotFound(c *gin.Context, msg string) {
	respondErr(c, http.StatusNotFound, http.StatusNotFound, msg)
}

// respondForbidden 返回 403 错误
func respondForbidden(c *gin.Context, msg string) {
	respondErr(c, http.StatusForbidden, http.StatusForbidden, msg)
}

// respondInternal 返回 500 错误
func respondInternal(c *gin.Context, msg string) {
	respondErr(c, http.StatusInternalServerError, http.StatusInternalServerError, msg)
}
