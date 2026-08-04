package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupConversationContextControllerDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	prevDB, prevLogDB, prevCtxDB := model.DB, model.LOG_DB, model.CONVERSATION_DB
	prevMain, prevLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB = db
	model.LOG_DB = db
	model.CONVERSATION_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.ConversationContext{}, &model.FavoriteConversationContext{}, &model.Log{}))

	t.Cleanup(func() {
		model.DB = prevDB
		model.LOG_DB = prevLogDB
		model.CONVERSATION_DB = prevCtxDB
		common.SetDatabaseTypes(prevMain, prevLog)
	})
}

func newConversationContextTestContext(userID int, role int) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/conversation-context", nil)
	c.Set("id", userID)
	c.Set("role", role)
	return c, recorder
}

func seedConversationContext(t *testing.T, requestID string, userID int) {
	t.Helper()
	require.NoError(t, model.UpsertConversationContext(context.Background(), &model.ConversationContext{
		RequestID:     requestID,
		UserID:        userID,
		CreatedAt:     common.GetTimestamp(),
		RequestPath:   "/v1/chat/completions",
		RelayFormat:   "openai",
		ModelName:     "deepseek-v4-flash",
		RequestBody:   `{"model":"deepseek-v4-flash"}`,
		ResponseBody:  `{"choices":[]}`,
		CaptureStatus: "completed",
	}))
}

type conversationContextEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decodeConversationContextResponse(t *testing.T, recorder *httptest.ResponseRecorder) conversationContextEnvelope {
	t.Helper()
	var env conversationContextEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &env))
	return env
}

func TestGetConversationContextsPermissionScoping(t *testing.T) {
	setupConversationContextControllerDB(t)
	seedConversationContext(t, "req-u1-a", 1)
	seedConversationContext(t, "req-u1-b", 1)
	seedConversationContext(t, "req-u2-a", 2)

	// regular user sees only their own contexts
	c, recorder := newConversationContextTestContext(1, common.RoleCommonUser)
	GetConversationContexts(c)
	assert.Equal(t, http.StatusOK, recorder.Code)
	env := decodeConversationContextResponse(t, recorder)
	assert.True(t, env.Success)
	var data struct {
		Total int                         `json:"total"`
		Items []model.ConversationContext `json:"items"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, 2, data.Total)
	assert.Len(t, data.Items, 2)

	// admin sees all users
	c, recorder = newConversationContextTestContext(10, common.RoleAdminUser)
	GetConversationContexts(c)
	env = decodeConversationContextResponse(t, recorder)
	assert.True(t, env.Success)
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, 3, data.Total)

	// admin filtered by user_id
	c, recorder = newConversationContextTestContext(10, common.RoleAdminUser)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/conversation-context?user_id=2", nil)
	GetConversationContexts(c)
	env = decodeConversationContextResponse(t, recorder)
	assert.True(t, env.Success)
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, 1, data.Total)
	assert.Equal(t, "req-u2-a", data.Items[0].RequestID)
}

func TestGetConversationContextDetailPermission(t *testing.T) {
	setupConversationContextControllerDB(t)
	seedConversationContext(t, "req-detail", 1)

	// owner can read
	c, recorder := newConversationContextTestContext(1, common.RoleCommonUser)
	c.Params = gin.Params{{Key: "request_id", Value: "req-detail"}}
	GetConversationContext(c)
	env := decodeConversationContextResponse(t, recorder)
	assert.True(t, env.Success)
	var data model.ConversationContext
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, "req-detail", data.RequestID)

	// other regular user is denied
	c, recorder = newConversationContextTestContext(2, common.RoleCommonUser)
	c.Params = gin.Params{{Key: "request_id", Value: "req-detail"}}
	GetConversationContext(c)
	env = decodeConversationContextResponse(t, recorder)
	assert.False(t, env.Success)
	assert.Contains(t, env.Message, "permission denied")

	// admin can read anyone
	c, recorder = newConversationContextTestContext(10, common.RoleAdminUser)
	c.Params = gin.Params{{Key: "request_id", Value: "req-detail"}}
	GetConversationContext(c)
	env = decodeConversationContextResponse(t, recorder)
	assert.True(t, env.Success)
}

func TestFavoriteConversationContextOnlyOwner(t *testing.T) {
	setupConversationContextControllerDB(t)
	seedConversationContext(t, "req-fav", 1)

	// owner favorites
	c, recorder := newConversationContextTestContext(1, common.RoleCommonUser)
	c.Params = gin.Params{{Key: "request_id", Value: "req-fav"}}
	FavoriteConversationContext(c)
	env := decodeConversationContextResponse(t, recorder)
	assert.True(t, env.Success)

	// other user cannot favorite
	c, recorder = newConversationContextTestContext(2, common.RoleCommonUser)
	c.Params = gin.Params{{Key: "request_id", Value: "req-fav"}}
	FavoriteConversationContext(c)
	env = decodeConversationContextResponse(t, recorder)
	assert.False(t, env.Success)

	// admin cannot favorite another user's context either
	c, recorder = newConversationContextTestContext(10, common.RoleAdminUser)
	c.Params = gin.Params{{Key: "request_id", Value: "req-fav"}}
	FavoriteConversationContext(c)
	env = decodeConversationContextResponse(t, recorder)
	assert.False(t, env.Success)

	// only the owner's favorite exists
	_, total, err := model.GetFavoriteConversationContexts(context.Background(), 1, "", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
}

func TestFavoriteConversationContextDetailSurvivesContextCleanup(t *testing.T) {
	setupConversationContextControllerDB(t)
	seedConversationContext(t, "req-survives-cleanup", 1)

	c, recorder := newConversationContextTestContext(1, common.RoleCommonUser)
	c.Params = gin.Params{{Key: "request_id", Value: "req-survives-cleanup"}}
	FavoriteConversationContext(c)
	env := decodeConversationContextResponse(t, recorder)
	require.True(t, env.Success)
	var created model.FavoriteConversationContext
	require.NoError(t, json.Unmarshal(env.Data, &created))
	require.NotZero(t, created.ID)

	_, err := model.DeleteOldConversationContextsByCreatedAt(context.Background(), common.GetTimestamp()+1)
	require.NoError(t, err)
	_, err = model.GetConversationContextByRequestID(context.Background(), "req-survives-cleanup")
	require.Error(t, err)

	c, recorder = newConversationContextTestContext(1, common.RoleCommonUser)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(created.ID))}}
	GetFavoriteConversationContext(c)
	env = decodeConversationContextResponse(t, recorder)
	require.True(t, env.Success)
	var favorite model.FavoriteConversationContext
	require.NoError(t, json.Unmarshal(env.Data, &favorite))
	assert.Equal(t, "req-survives-cleanup", favorite.RequestID)
	assert.Equal(t, model.FavoriteConversationPayload(`{"model":"deepseek-v4-flash"}`), favorite.RequestBody)
}

func TestFavoriteConversationContextListDeleteScoped(t *testing.T) {
	setupConversationContextControllerDB(t)
	seedConversationContext(t, "req-fav1", 1)
	seedConversationContext(t, "req-fav2", 2)

	require.NoError(t, model.CreateFavoriteConversationContext(context.Background(), &model.FavoriteConversationContext{
		UserID:    1,
		RequestID: "req-fav1",
	}))
	require.NoError(t, model.CreateFavoriteConversationContext(context.Background(), &model.FavoriteConversationContext{
		UserID:    2,
		RequestID: "req-fav2",
	}))

	// each user only sees their own favorites
	c, recorder := newConversationContextTestContext(1, common.RoleCommonUser)
	GetFavoriteConversationContexts(c)
	env := decodeConversationContextResponse(t, recorder)
	assert.True(t, env.Success)
	var data struct {
		Total int                                 `json:"total"`
		Items []model.FavoriteConversationContext `json:"items"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, 1, data.Total)
	assert.Equal(t, "req-fav1", data.Items[0].RequestID)

	// user2 cannot delete user1's favorite
	c, recorder = newConversationContextTestContext(2, common.RoleCommonUser)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	DeleteFavoriteConversationContext(c)
	env = decodeConversationContextResponse(t, recorder)
	assert.False(t, env.Success)

	// user1 can delete it
	c, recorder = newConversationContextTestContext(1, common.RoleCommonUser)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	DeleteFavoriteConversationContext(c)
	env = decodeConversationContextResponse(t, recorder)
	assert.True(t, env.Success)
}
