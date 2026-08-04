package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetConversationContexts lists conversation contexts (DB A). Regular users
// are always scoped to themselves; admin/root can pass user_id to view any
// user (or omit it to view all). Items include the is_favorite flag computed
// against the current user's favorites (DB B).
func GetConversationContexts(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	currentUserID := c.GetInt("id")
	role := c.GetInt("role")
	queryUserID := 0
	if role >= common.RoleAdminUser {
		queryUserID, _ = strconv.Atoi(c.Query("user_id"))
	} else {
		queryUserID = currentUserID
	}
	requestID := c.Query("request_id")
	items, total, err := model.GetConversationContexts(c, currentUserID, queryUserID, requestID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// GetConversationContext returns one conversation context by request_id.
// Regular users may only view their own; admin/root may view any.
func GetConversationContext(c *gin.Context) {
	requestID := c.Param("request_id")
	record, err := model.GetConversationContextByRequestID(c, requestID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if c.GetInt("role") < common.RoleAdminUser && record.UserID != c.GetInt("id") {
		common.ApiErrorMsg(c, "permission denied")
		return
	}
	isFavorite, err := model.IsConversationContextFavorite(c, c.GetInt("id"), record.RequestID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	record.IsFavorite = isFavorite
	common.ApiSuccess(c, record)
}

// FavoriteConversationContext snapshots a conversation context into the
// current user's favorites (DB B). Only the context owner can favorite it.
func FavoriteConversationContext(c *gin.Context) {
	requestID := c.Param("request_id")
	record, err := model.GetConversationContextByRequestID(c, requestID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	currentUserID := c.GetInt("id")
	if record.UserID != currentUserID {
		common.ApiErrorMsg(c, "only the owner can favorite the conversation context")
		return
	}
	fav := &model.FavoriteConversationContext{
		UserID:         currentUserID,
		RequestID:      record.RequestID,
		SourceUserID:   record.UserID,
		LogID:          record.LogID,
		CreatedAt:      record.CreatedAt,
		FavoritedAt:    common.GetTimestamp(),
		RequestPath:    record.RequestPath,
		RelayFormat:    record.RelayFormat,
		ModelName:      record.ModelName,
		RequestBody:    model.FavoriteConversationPayload(record.RequestBody),
		ResponseBody:   model.FavoriteConversationPayload(record.ResponseBody),
		ResponseStatus: record.ResponseStatus,
		IsStream:       record.IsStream,
		CaptureStatus:  record.CaptureStatus,
	}
	if err := model.CreateFavoriteConversationContext(c, fav); err != nil {
		common.ApiError(c, err)
		return
	}
	fav.IsFavorite = true
	common.ApiSuccess(c, fav)
}

// GetFavoriteConversationContexts lists the current user's favorites (DB B).
func GetFavoriteConversationContexts(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	requestID := c.Query("request_id")
	items, total, err := model.GetFavoriteConversationContexts(c, c.GetInt("id"), requestID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// GetFavoriteConversationContext returns one of the current user's favorites.
func GetFavoriteConversationContext(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	fav, err := model.GetFavoriteConversationContextByID(c, c.GetInt("id"), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, fav)
}

// DeleteFavoriteConversationContext removes one of the current user's favorites.
func DeleteFavoriteConversationContext(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rows, err := model.DeleteFavoriteConversationContext(c, c.GetInt("id"), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if rows == 0 {
		common.ApiErrorMsg(c, "favorite not found")
		return
	}
	common.ApiSuccess(c, nil)
}
