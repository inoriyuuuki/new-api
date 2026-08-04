package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupConversationContextDB points CONVERSATION_DB at an in-memory SQLite
// database (the model TestMain already wires DB/LOG_DB to another shared
// in-memory connection) and migrates both new tables.
func setupConversationContextDB(t *testing.T) {
	t.Helper()
	if CONVERSATION_DB == nil {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		CONVERSATION_DB = db
	}
	require.NoError(t, CONVERSATION_DB.AutoMigrate(&ConversationContext{}))
	require.NoError(t, DB.AutoMigrate(&FavoriteConversationContext{}))
	t.Cleanup(func() {
		CONVERSATION_DB.Exec("DELETE FROM conversation_contexts")
		DB.Exec("DELETE FROM favorite_conversation_contexts")
		DB.Exec("DELETE FROM logs")
	})
}

func insertTestLog(t *testing.T, logType int, requestID string) *Log {
	t.Helper()
	log := &Log{
		UserId:    1,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		RequestId: requestID,
		Content:   "test log",
	}
	require.NoError(t, LOG_DB.Create(log).Error)
	return log
}

func TestUpsertConversationContextIdempotent(t *testing.T) {
	setupConversationContextDB(t)
	ctx := context.Background()

	record := &ConversationContext{
		RequestID:      "req-idem",
		UserID:         1,
		CreatedAt:      common.GetTimestamp(),
		RequestPath:    "/v1/chat/completions",
		RelayFormat:    "openai",
		ModelName:      "deepseek-v4-flash",
		RequestBody:    `{"model":"deepseek-v4-flash"}`,
		ResponseBody:   `{"choices":[]}`,
		ResponseStatus: 200,
		IsStream:       false,
		CaptureStatus:  "completed",
	}
	require.NoError(t, UpsertConversationContext(ctx, record))
	require.NoError(t, UpsertConversationContext(ctx, record))

	var total int64
	require.NoError(t, CONVERSATION_DB.Model(&ConversationContext{}).Where("request_id = ?", "req-idem").Count(&total).Error)
	assert.Equal(t, int64(1), total)
}

func TestUpsertConversationContextLogIDPriority(t *testing.T) {
	setupConversationContextDB(t)
	ctx := context.Background()

	// 1) no log yet -> log_id stays 0
	record := &ConversationContext{RequestID: "req-prio", UserID: 1, CreatedAt: common.GetTimestamp()}
	require.NoError(t, UpsertConversationContext(ctx, record))
	assert.Equal(t, 0, record.LogID)

	// 2) only error log -> linked to the error id
	errorLog := insertTestLog(t, LogTypeError, "req-prio")
	record2 := &ConversationContext{RequestID: "req-prio", UserID: 1, CreatedAt: common.GetTimestamp()}
	require.NoError(t, UpsertConversationContext(ctx, record2))
	got, err := GetConversationContextByRequestID(ctx, "req-prio")
	require.NoError(t, err)
	assert.Equal(t, errorLog.Id, got.LogID)

	// 3) consume log appears -> upgraded to the consume id even though the
	// caller passes the (older) error id as a hint.
	consumeLog := insertTestLog(t, LogTypeConsume, "req-prio")
	record3 := &ConversationContext{RequestID: "req-prio", UserID: 1, CreatedAt: common.GetTimestamp(), LogID: errorLog.Id}
	require.NoError(t, UpsertConversationContext(ctx, record3))
	got, err = GetConversationContextByRequestID(ctx, "req-prio")
	require.NoError(t, err)
	assert.Equal(t, consumeLog.Id, got.LogID)

	// 4) a later error log must not overwrite the consume id.
	_ = insertTestLog(t, LogTypeError, "req-prio")
	record4 := &ConversationContext{RequestID: "req-prio", UserID: 1, CreatedAt: common.GetTimestamp()}
	require.NoError(t, UpsertConversationContext(ctx, record4))
	got, err = GetConversationContextByRequestID(ctx, "req-prio")
	require.NoError(t, err)
	assert.Equal(t, consumeLog.Id, got.LogID)
}

func TestUpsertConversationContextLatestOther(t *testing.T) {
	setupConversationContextDB(t)
	ctx := context.Background()

	firstError := insertTestLog(t, LogTypeError, "req-latest")
	secondError := insertTestLog(t, LogTypeError, "req-latest")
	require.True(t, secondError.Id > firstError.Id)

	record := &ConversationContext{RequestID: "req-latest", UserID: 1, CreatedAt: common.GetTimestamp()}
	require.NoError(t, UpsertConversationContext(ctx, record))
	got, err := GetConversationContextByRequestID(ctx, "req-latest")
	require.NoError(t, err)
	assert.Equal(t, secondError.Id, got.LogID)
}

func TestCreateLogBackfillsConversationLogIDWithConsumePriority(t *testing.T) {
	setupConversationContextDB(t)
	ctx := context.Background()

	// The context wins the initial race and is persisted before any log.
	record := &ConversationContext{RequestID: "req-backfill", UserID: 1, CreatedAt: common.GetTimestamp()}
	require.NoError(t, UpsertConversationContext(ctx, record))
	got, err := GetConversationContextByRequestID(ctx, "req-backfill")
	require.NoError(t, err)
	assert.Equal(t, 0, got.LogID)

	// An error log links first.
	errorLog := &Log{
		UserId:    1,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeError,
		RequestId: "req-backfill",
		Content:   "error",
	}
	require.NoError(t, createLog(errorLog))
	require.Eventually(t, func() bool {
		got, err = GetConversationContextByRequestID(ctx, "req-backfill")
		return err == nil && got.LogID == errorLog.Id
	}, 2*time.Second, 10*time.Millisecond)

	// A consume log upgrades the association.
	consumeLog := &Log{
		UserId:    1,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeConsume,
		RequestId: "req-backfill",
		Content:   "consume",
	}
	require.NoError(t, createLog(consumeLog))
	require.Eventually(t, func() bool {
		got, err = GetConversationContextByRequestID(ctx, "req-backfill")
		return err == nil && got.LogID == consumeLog.Id
	}, 2*time.Second, 10*time.Millisecond)

	// A later error must not downgrade an already-linked consume log.
	laterError := &Log{
		UserId:    1,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeError,
		RequestId: "req-backfill",
		Content:   "later error",
	}
	require.NoError(t, createLog(laterError))
	require.NoError(t, BackfillConversationLogID(ctx, "req-backfill"))
	got, err = GetConversationContextByRequestID(ctx, "req-backfill")
	require.NoError(t, err)
	assert.Equal(t, consumeLog.Id, got.LogID)
}

func TestUpsertConversationContextClickHouseKeepsZero(t *testing.T) {
	setupConversationContextDB(t)
	ctx := context.Background()

	_ = insertTestLog(t, LogTypeConsume, "req-clickhouse")

	previousType := common.LogDatabaseType()
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)
	t.Cleanup(func() { common.SetLogDatabaseType(previousType) })

	record := &ConversationContext{RequestID: "req-clickhouse", UserID: 1, CreatedAt: common.GetTimestamp()}
	require.NoError(t, UpsertConversationContext(ctx, record))
	got, err := GetConversationContextByRequestID(ctx, "req-clickhouse")
	require.NoError(t, err)
	assert.Equal(t, 0, got.LogID)
}

func TestGetConversationContextsPaginationAndFavorite(t *testing.T) {
	setupConversationContextDB(t)
	ctx := context.Background()

	for i, uid := range []int{1, 1, 2} {
		record := &ConversationContext{
			RequestID:    "req-list-" + string(rune('a'+i)),
			UserID:       uid,
			CreatedAt:    common.GetTimestamp(),
			ModelName:    "deepseek-v4-flash",
			RequestBody:  `{"messages":[{"role":"user","content":"large payload"}]}`,
			ResponseBody: `{"choices":[{"message":{"role":"assistant","content":"large payload"}}]}`,
			IsFavorite:   false,
		}
		require.NoError(t, UpsertConversationContext(ctx, record))
	}

	// user1 favorites the first request.
	require.NoError(t, CreateFavoriteConversationContext(ctx, &FavoriteConversationContext{
		UserID:    1,
		RequestID: "req-list-a",
	}))

	// regular user scoped to self
	items, total, err := GetConversationContexts(ctx, 1, 1, "", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, items, 2)
	byReq := map[string]*ConversationContext{}
	for _, it := range items {
		byReq[it.RequestID] = it
	}
	assert.True(t, byReq["req-list-a"].IsFavorite)
	assert.False(t, byReq["req-list-b"].IsFavorite)
	assert.Empty(t, byReq["req-list-a"].RequestBody)
	assert.Empty(t, byReq["req-list-a"].ResponseBody)
	detail, err := GetConversationContextByRequestID(ctx, "req-list-a")
	require.NoError(t, err)
	assert.NotEmpty(t, detail.RequestBody)
	assert.NotEmpty(t, detail.ResponseBody)

	// admin sees all users
	items, total, err = GetConversationContexts(ctx, 1, 0, "", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)

	// pagination
	items, total, err = GetConversationContexts(ctx, 1, 1, "", 0, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, items, 1)

	// request_id filter
	items, total, err = GetConversationContexts(ctx, 1, 0, "req-list-c", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-list-c", items[0].RequestID)
}

func TestFavoriteConversationContextCRUD(t *testing.T) {
	setupConversationContextDB(t)
	ctx := context.Background()

	fav := &FavoriteConversationContext{
		UserID:       1,
		RequestID:    "req-fav",
		SourceUserID: 1,
		LogID:        42,
		CreatedAt:    common.GetTimestamp(),
		FavoritedAt:  common.GetTimestamp(),
		ModelName:    "deepseek-v4-flash",
		RequestBody:  `{"q":1}`,
		ResponseBody: `{"answer":1}`,
	}
	require.NoError(t, CreateFavoriteConversationContext(ctx, fav))
	// idempotent duplicate
	require.NoError(t, CreateFavoriteConversationContext(ctx, fav))

	items, total, err := GetFavoriteConversationContexts(ctx, 1, "", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, items, 1)
	assert.True(t, items[0].IsFavorite)
	assert.Equal(t, 42, items[0].LogID)
	assert.Empty(t, items[0].RequestBody)
	assert.Empty(t, items[0].ResponseBody)

	// another user cannot see it
	otherItems, total, err := GetFavoriteConversationContexts(ctx, 2, "", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, otherItems)

	got, err := GetFavoriteConversationContextByID(ctx, 1, int(fav.ID))
	require.NoError(t, err)
	assert.Equal(t, "req-fav", got.RequestID)
	assert.Equal(t, FavoriteConversationPayload(`{"q":1}`), got.RequestBody)
	assert.Equal(t, FavoriteConversationPayload(`{"answer":1}`), got.ResponseBody)
	_, err = GetFavoriteConversationContextByID(ctx, 2, int(fav.ID))
	require.Error(t, err)

	rows, err := DeleteFavoriteConversationContext(ctx, 2, int(fav.ID))
	require.NoError(t, err)
	assert.Equal(t, int64(0), rows)
	rows, err = DeleteFavoriteConversationContext(ctx, 1, int(fav.ID))
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)
}

func TestDeleteOldConversationContextsByCreatedAt(t *testing.T) {
	setupConversationContextDB(t)
	ctx := context.Background()

	now := common.GetTimestamp()
	require.NoError(t, UpsertConversationContext(ctx, &ConversationContext{RequestID: "req-old", UserID: 1, CreatedAt: now - 10000}))
	require.NoError(t, UpsertConversationContext(ctx, &ConversationContext{RequestID: "req-new", UserID: 1, CreatedAt: now}))
	require.NoError(t, CreateFavoriteConversationContext(ctx, &FavoriteConversationContext{
		UserID:    1,
		RequestID: "req-old",
	}))

	rows, err := DeleteOldConversationContextsByCreatedAt(ctx, now-5000)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)

	_, err = GetConversationContextByRequestID(ctx, "req-old")
	require.Error(t, err)
	got, err := GetConversationContextByRequestID(ctx, "req-new")
	require.NoError(t, err)
	assert.NotNil(t, got)

	// favorites survive cleanup
	_, total, err := GetFavoriteConversationContexts(ctx, 1, "", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
}
