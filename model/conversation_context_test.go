package model

import (
	"context"
	"encoding/json"
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

	streamStatus := `{"status":"error","end_reason":"client_gone","end_error":"context canceled"}`
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
		StreamStatus:   streamStatus,
	}
	require.NoError(t, UpsertConversationContext(ctx, record))
	require.NoError(t, UpsertConversationContext(ctx, record))

	var total int64
	require.NoError(t, CONVERSATION_DB.Model(&ConversationContext{}).Where("request_id = ?", "req-idem").Count(&total).Error)
	assert.Equal(t, int64(1), total)

	// The stream status snapshot survives the upsert round-trip.
	got, err := GetConversationContextByRequestID(ctx, "req-idem")
	require.NoError(t, err)
	assert.Equal(t, streamStatus, got.StreamStatus)
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

func TestMarkLogsHasContext(t *testing.T) {
	setupConversationContextDB(t)
	ctx := context.Background()

	// Two contexts exist in DB A: one matches a log, one is an orphan.
	require.NoError(t, UpsertConversationContext(ctx, &ConversationContext{
		RequestID: "req-has-ctx",
		UserID:    1,
		CreatedAt: common.GetTimestamp(),
	}))
	require.NoError(t, UpsertConversationContext(ctx, &ConversationContext{
		RequestID: "req-orphan",
		UserID:    2,
		CreatedAt: common.GetTimestamp(),
	}))

	// Stale values are pre-set on purpose: flags must be reset to false first,
	// so a reused Log object can never keep an old true flag.
	logs := []*Log{
		{Id: 1, RequestId: "req-has-ctx", HasContext: true}, // exists -> true
		{Id: 2, RequestId: "req-missing", HasContext: true}, // not exists -> reset to false
		{Id: 3, RequestId: "", HasContext: true},            // empty id -> reset to false, no crash
		{Id: 4, RequestId: "req-has-ctx"},                   // duplicate id -> true
		{Id: 5},                                             // nil request id -> false
	}
	markLogsHasContext(logs)

	require.Len(t, logs, 5)
	assert.True(t, logs[0].HasContext, "existing request_id must be true")
	assert.False(t, logs[1].HasContext, "missing request_id must be false")
	assert.False(t, logs[2].HasContext, "empty request_id must be false")
	assert.True(t, logs[3].HasContext, "duplicate request_id must also be true")
	assert.False(t, logs[4].HasContext, "zero request_id must be false")

	// Nil slice is a no-op.
	markLogsHasContext(nil)
}

// TestMarkLogsHasContextClosedDB covers a real query error (closed SQLite
// connection), not just a nil handle: the mark helper must not panic and must
// leave every flag false (best-effort).
func TestMarkLogsHasContextClosedDB(t *testing.T) {
	saved := CONVERSATION_DB
	t.Cleanup(func() { CONVERSATION_DB = saved })

	ctxDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, ctxDB.AutoMigrate(&ConversationContext{}))
	sqlDB, err := ctxDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	CONVERSATION_DB = ctxDB

	logs := []*Log{
		{Id: 1, RequestId: "req-query-fail", HasContext: true}, // stale value must be reset
		{Id: 2, RequestId: "req-query-fail"},
	}
	markLogsHasContext(logs)
	require.Len(t, logs, 2)
	assert.False(t, logs[0].HasContext, "query error must reset stale true to false")
	assert.False(t, logs[1].HasContext)
}

// TestGetAllLogsSurvivesContextDBFailure verifies that a failing DB A does not
// break the usage log list: logs are still returned successfully with all
// has_context flags false.
func TestGetAllLogsSurvivesContextDBFailure(t *testing.T) {
	now := common.GetTimestamp()
	log := &Log{UserId: 1, Username: "u1", CreatedAt: now, Type: LogTypeConsume, RequestId: "req-db-fail", Content: "x"}
	require.NoError(t, LOG_DB.Create(log).Error)
	t.Cleanup(func() {
		LOG_DB.Where("request_id = ?", "req-db-fail").Delete(&Log{})
	})

	saved := CONVERSATION_DB
	t.Cleanup(func() { CONVERSATION_DB = saved })
	ctxDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, ctxDB.AutoMigrate(&ConversationContext{}))
	sqlDB, err := ctxDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	CONVERSATION_DB = ctxDB

	logs, total, err := GetAllLogs(LogTypeConsume, 0, 0, "", "", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.False(t, logs[0].HasContext, "DB A failure must leave flags false without failing the list")
}

func TestMarkLogsHasContextNilDB(t *testing.T) {
	saved := CONVERSATION_DB
	CONVERSATION_DB = nil
	t.Cleanup(func() { CONVERSATION_DB = saved })

	// Stale true values must be reset to false even when DB A is nil: the
	// reset pass runs before any DB A access.
	logs := []*Log{
		{Id: 1, RequestId: "req-any", HasContext: true},
		{Id: 2, RequestId: "req-any", HasContext: true},
	}
	// Must not panic and must leave flags false when DB A is unavailable.
	markLogsHasContext(logs)
	assert.False(t, logs[0].HasContext, "stale true must be reset even with nil DB A")
	assert.False(t, logs[1].HasContext)
}

func TestGetAllLogsMarksHasContext(t *testing.T) {
	setupConversationContextDB(t)
	ctx := context.Background()

	now := common.GetTimestamp()
	// Two consume logs: one has a context in DB A, the other does not.
	logA := &Log{UserId: 1, Username: "u1", CreatedAt: now, Type: LogTypeConsume, RequestId: "req-all-logs-a", Content: "a"}
	logB := &Log{UserId: 1, Username: "u1", CreatedAt: now, Type: LogTypeConsume, RequestId: "req-all-logs-b", Content: "b"}
	require.NoError(t, LOG_DB.Create(logA).Error)
	require.NoError(t, LOG_DB.Create(logB).Error)
	require.NoError(t, UpsertConversationContext(ctx, &ConversationContext{
		RequestID: "req-all-logs-a",
		UserID:    1,
		CreatedAt: now,
	}))

	logs, total, err := GetAllLogs(LogTypeConsume, 0, 0, "", "", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	byID := map[string]*Log{}
	for _, l := range logs {
		byID[l.RequestId] = l
	}
	require.Contains(t, byID, "req-all-logs-a")
	require.Contains(t, byID, "req-all-logs-b")
	assert.True(t, byID["req-all-logs-a"].HasContext)
	assert.False(t, byID["req-all-logs-b"].HasContext)

	// HasContext must not be persisted: reloading from LOG_DB yields false.
	var reloaded Log
	require.NoError(t, LOG_DB.First(&reloaded, logA.Id).Error)
	assert.False(t, reloaded.HasContext, "has_context must not be written to the logs table")
}

func TestGetUserLogsMarksHasContext(t *testing.T) {
	setupConversationContextDB(t)
	ctx := context.Background()

	now := common.GetTimestamp()
	logA := &Log{UserId: 1, Username: "u1", CreatedAt: now, Type: LogTypeConsume, RequestId: "req-user-logs-a", Content: "a"}
	logB := &Log{UserId: 1, Username: "u1", CreatedAt: now, Type: LogTypeConsume, RequestId: "req-user-logs-b", Content: "b"}
	require.NoError(t, LOG_DB.Create(logA).Error)
	require.NoError(t, LOG_DB.Create(logB).Error)
	require.NoError(t, UpsertConversationContext(ctx, &ConversationContext{
		RequestID: "req-user-logs-b",
		UserID:    1,
		CreatedAt: now,
	}))

	logs, total, err := GetUserLogs(1, LogTypeConsume, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	byID := map[string]*Log{}
	for _, l := range logs {
		byID[l.RequestId] = l
	}
	require.Contains(t, byID, "req-user-logs-a")
	require.Contains(t, byID, "req-user-logs-b")
	assert.False(t, byID["req-user-logs-a"].HasContext)
	assert.True(t, byID["req-user-logs-b"].HasContext)

	// Non-conversation log types (manage/login) are always false.
	manage := &Log{UserId: 1, Username: "u1", CreatedAt: now, Type: LogTypeManage, RequestId: "req-manage", Content: "m"}
	require.NoError(t, LOG_DB.Create(manage).Error)
	mlogs, mtotal, err := GetUserLogs(1, LogTypeManage, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), mtotal)
	require.Len(t, mlogs, 1)
	assert.False(t, mlogs[0].HasContext)
}

func TestLogHasContextJSONExplicit(t *testing.T) {
	data, err := json.Marshal(&Log{})
	require.NoError(t, err)
	// False must be emitted explicitly (no omitempty) so the frontend can rely
	// on the field being present.
	assert.Contains(t, string(data), `"has_context":false`)
}
