package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// contextDBPath is the fixed per-node local SQLite file used to store
// conversation context (DB A). It intentionally does not read any environment
// variable: every node owns its own local copy so large request/response
// bodies never pollute the main or log databases.
const contextDBPath = "conversation-context.db?_busy_timeout=30000"

// ConversationContext (DB A, local SQLite) captures the raw request/response
// of a relay call and links it to the usage log entry via RequestID/LogID.
type ConversationContext struct {
	ID             uint   `gorm:"primary_key" json:"id"`
	LogID          int    `gorm:"index;default:0" json:"log_id"`
	RequestID      string `gorm:"type:varchar(64);uniqueIndex" json:"request_id"`
	UserID         int    `gorm:"index" json:"user_id"`
	CreatedAt      int64  `gorm:"index" json:"created_at"`
	RequestPath    string `gorm:"type:varchar(255)" json:"request_path"`
	RelayFormat    string `gorm:"type:varchar(64)" json:"relay_format"`
	ModelName      string `gorm:"type:varchar(128);index" json:"model_name"`
	RequestBody    string `gorm:"type:text" json:"request_body"`
	ResponseBody   string `gorm:"type:text" json:"response_body"`
	ResponseStatus int    `json:"response_status"`
	IsStream       bool   `json:"is_stream"`
	CaptureStatus  string `gorm:"type:varchar(32)" json:"capture_status"`
	// IsFavorite is computed at read time from DB B and never persisted.
	IsFavorite bool `gorm:"-" json:"is_favorite"`
}

// FavoriteConversationPayload stores large captured bodies in DB B. MySQL's
// plain TEXT is limited to 64KB, so favorites use LONGTEXT there while SQLite
// and PostgreSQL use their unbounded TEXT type.
type FavoriteConversationPayload string

func (FavoriteConversationPayload) GormDataType() string {
	return "text"
}

func (FavoriteConversationPayload) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db != nil && db.Dialector.Name() == "mysql" {
		return "LONGTEXT"
	}
	return "TEXT"
}

// FavoriteConversationContext (DB B, main database) is a full immutable
// snapshot of a conversation context. It survives log/context cleanup and is
// only removed manually by its owner.
type FavoriteConversationContext struct {
	ID             uint                        `gorm:"primary_key" json:"id"`
	UserID         int                         `gorm:"index;uniqueIndex:idx_fav_user_request,priority:1" json:"user_id"`
	RequestID      string                      `gorm:"type:varchar(64);uniqueIndex:idx_fav_user_request,priority:2" json:"request_id"`
	SourceUserID   int                         `json:"source_user_id"`
	LogID          int                         `gorm:"default:0" json:"log_id"`
	CreatedAt      int64                       `json:"created_at"`
	FavoritedAt    int64                       `gorm:"index" json:"favorited_at"`
	RequestPath    string                      `gorm:"type:varchar(255)" json:"request_path"`
	RelayFormat    string                      `gorm:"type:varchar(64)" json:"relay_format"`
	ModelName      string                      `gorm:"type:varchar(128)" json:"model_name"`
	RequestBody    FavoriteConversationPayload `json:"request_body"`
	ResponseBody   FavoriteConversationPayload `json:"response_body"`
	ResponseStatus int                         `json:"response_status"`
	IsStream       bool                        `json:"is_stream"`
	CaptureStatus  string                      `gorm:"type:varchar(32)" json:"capture_status"`
	IsFavorite     bool                        `gorm:"-" json:"is_favorite"`
}

// CONVERSATION_DB is the per-node local conversation context database (DB A).
var CONVERSATION_DB *gorm.DB

// InitContextDB opens the fixed local SQLite file and migrates
// ConversationContext. Unlike the main/log databases, this runs on every node
// (master and slave) because each node owns its local context copy. Favorite
// snapshots live in the main database and follow the existing master-only
// migration strategy (see migrateDB/migrateDBFast).
func InitContextDB() error {
	db, err := gorm.Open(sqlite.Open(contextDBPath), &gorm.Config{
		PrepareStmt: true, // precompile SQL
	})
	if err != nil {
		return err
	}
	if common.DebugEnabled {
		db = db.Debug()
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	// A single connection serializes writes and avoids SQLite lock contention;
	// the DSN already enables a 30s busy timeout.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	CONVERSATION_DB = db
	common.SysLog("conversation context database migration started")
	return CONVERSATION_DB.AutoMigrate(&ConversationContext{})
}

// resolveConversationLogID returns the authoritative log id for a request:
// the real LogTypeConsume id wins, otherwise the latest other (e.g. error)
// log id. ClickHouse rows have no stable real id (id Int64 DEFAULT 0), so it
// returns 0 and only request_id is retained.
func resolveConversationLogID(ctx context.Context, requestID string) (int, error) {
	if LOG_DB == nil || requestID == "" {
		return 0, nil
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return 0, nil
	}
	var consumeID int
	err := LOG_DB.WithContext(ctx).Model(&Log{}).
		Where("request_id = ? AND type = ?", requestID, LogTypeConsume).
		Order("id DESC").Limit(1).Select("id").Scan(&consumeID).Error
	if err != nil {
		return 0, err
	}
	if consumeID > 0 {
		return consumeID, nil
	}
	var latestID int
	err = LOG_DB.WithContext(ctx).Model(&Log{}).
		Where("request_id = ?", requestID).
		Order("id DESC").Limit(1).Select("id").Scan(&latestID).Error
	if err != nil {
		return 0, err
	}
	return latestID, nil
}

// UpsertConversationContext creates or updates a ConversationContext by
// request_id. It re-queries the logs table so a context written before the log
// still gets linked; the consume log id always takes priority over error/other
// ids, and a later error never overwrites an already associated consume id.
// With ClickHouse the log id stays 0 (request_id only).
func UpsertConversationContext(ctx context.Context, record *ConversationContext) error {
	if CONVERSATION_DB == nil {
		return errors.New("conversation context database is not initialized")
	}
	if record == nil || record.RequestID == "" {
		return errors.New("request_id is required")
	}
	if record.UserID <= 0 {
		return errors.New("user_id is required")
	}
	if record.CreatedAt == 0 {
		record.CreatedAt = common.GetTimestamp()
	}

	var existing ConversationContext
	err := CONVERSATION_DB.WithContext(ctx).Where("request_id = ?", record.RequestID).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	logID, resolveErr := resolveConversationLogID(ctx, record.RequestID)
	if resolveErr != nil {
		// Log DB unavailable: preserve the best known association instead of
		// clobbering it with a lower-priority id.
		logID = record.LogID
		if existing.LogID > 0 {
			logID = existing.LogID
		}
	} else {
		if logID == 0 {
			// No authoritative id found: never downgrade an already-linked
			// context with a lower-priority caller hint.
			if existing.LogID > 0 {
				logID = existing.LogID
			} else {
				logID = record.LogID
			}
		}
		if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
			logID = 0
		}
	}
	record.LogID = logID

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CONVERSATION_DB.WithContext(ctx).Create(record).Error
	}

	updates := map[string]interface{}{
		"log_id":          record.LogID,
		"request_path":    record.RequestPath,
		"relay_format":    record.RelayFormat,
		"model_name":      record.ModelName,
		"request_body":    record.RequestBody,
		"response_body":   record.ResponseBody,
		"response_status": record.ResponseStatus,
		"is_stream":       record.IsStream,
		"capture_status":  record.CaptureStatus,
	}
	return CONVERSATION_DB.WithContext(ctx).Model(&ConversationContext{}).
		Where("id = ?", existing.ID).Updates(updates).Error
}

// BackfillConversationLogID refreshes the log_id of an existing context by
// request_id. Call it after a log row is created (e.g. after createLog) so a
// context that was written before its log still gets linked. It follows the
// same consume-first priority and never downgrades an existing consume id.
func BackfillConversationLogID(ctx context.Context, requestID string) error {
	if CONVERSATION_DB == nil {
		return errors.New("conversation context database is not initialized")
	}
	if requestID == "" {
		return nil
	}
	var existing ConversationContext
	err := CONVERSATION_DB.WithContext(ctx).Where("request_id = ?", requestID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	logID, err := resolveConversationLogID(ctx, requestID)
	if err != nil {
		return err
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		logID = 0
	}
	if logID <= 0 || logID == existing.LogID {
		return nil
	}
	return CONVERSATION_DB.WithContext(ctx).Model(&ConversationContext{}).
		Where("id = ?", existing.ID).Update("log_id", logID).Error
}

// GetConversationContexts pages through DB A. queryUserID <= 0 means all
// users (admin/root); otherwise the result is scoped to that user. is_favorite
// is computed against DB B for currentUserID.
func GetConversationContexts(ctx context.Context, currentUserID int, queryUserID int, requestID string, startIdx int, pageSize int) ([]*ConversationContext, int64, error) {
	if CONVERSATION_DB == nil {
		return nil, 0, errors.New("conversation context database is not initialized")
	}
	tx := CONVERSATION_DB.WithContext(ctx).Model(&ConversationContext{})
	if queryUserID > 0 {
		tx = tx.Where("user_id = ?", queryUserID)
	}
	if requestID != "" {
		tx = tx.Where("request_id = ?", requestID)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*ConversationContext
	// Request/response payloads can be tens of megabytes. Keep collection
	// responses metadata-only and load the full payload exclusively from the
	// detail endpoint.
	if err := tx.Select(
		"id", "log_id", "request_id", "user_id", "created_at", "request_path",
		"relay_format", "model_name", "response_status", "is_stream", "capture_status",
	).Order("id DESC").Limit(pageSize).Offset(startIdx).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	if len(items) > 0 && currentUserID > 0 {
		favSet, err := favoriteRequestIDSet(ctx, currentUserID, items)
		if err != nil {
			return nil, 0, err
		}
		for _, item := range items {
			item.IsFavorite = favSet[item.RequestID]
		}
	}
	return items, total, nil
}

// GetConversationContextByRequestID returns one context from DB A.
func GetConversationContextByRequestID(ctx context.Context, requestID string) (*ConversationContext, error) {
	if CONVERSATION_DB == nil {
		return nil, errors.New("conversation context database is not initialized")
	}
	var record ConversationContext
	err := CONVERSATION_DB.WithContext(ctx).Where("request_id = ?", requestID).First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// IsConversationContextFavorite reports whether currentUserID favorited the
// given request in DB B.
func IsConversationContextFavorite(ctx context.Context, userID int, requestID string) (bool, error) {
	if userID <= 0 || requestID == "" {
		return false, nil
	}
	var count int64
	err := DB.WithContext(ctx).Model(&FavoriteConversationContext{}).
		Where("user_id = ? AND request_id = ?", userID, requestID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteOldConversationContextsByCreatedAt removes DB A rows older than
// targetTimestamp. It is invoked by the log cleanup task after the logs pass
// so contexts are cleaned together with their usage logs. Favorites in DB B
// are intentionally never touched here.
func DeleteOldConversationContextsByCreatedAt(ctx context.Context, targetTimestamp int64) (int64, error) {
	if CONVERSATION_DB == nil {
		return 0, errors.New("conversation context database is not initialized")
	}
	result := CONVERSATION_DB.WithContext(ctx).Where("created_at < ?", targetTimestamp).Delete(&ConversationContext{})
	return result.RowsAffected, result.Error
}

// CreateFavoriteConversationContext snapshots a context into DB B. Favoriting
// the same request twice is idempotent (keeps the original snapshot).
func CreateFavoriteConversationContext(ctx context.Context, fav *FavoriteConversationContext) error {
	if fav == nil || fav.UserID <= 0 || fav.RequestID == "" {
		return errors.New("invalid favorite conversation context")
	}
	if fav.FavoritedAt == 0 {
		fav.FavoritedAt = common.GetTimestamp()
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "request_id"}},
		DoNothing: true,
	}).Create(fav).Error
}

// GetFavoriteConversationContexts pages through the current user's favorites
// in DB B.
func GetFavoriteConversationContexts(ctx context.Context, userID int, requestID string, startIdx int, pageSize int) ([]*FavoriteConversationContext, int64, error) {
	tx := DB.WithContext(ctx).Model(&FavoriteConversationContext{})
	if userID > 0 {
		tx = tx.Where("user_id = ?", userID)
	}
	if requestID != "" {
		tx = tx.Where("request_id = ?", requestID)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*FavoriteConversationContext
	// Favorites retain complete snapshots in DB B, but list responses stay
	// metadata-only so paging does not repeatedly transfer large bodies.
	if err := tx.Select(
		"id", "user_id", "request_id", "source_user_id", "log_id", "created_at",
		"favorited_at", "request_path", "relay_format", "model_name",
		"response_status", "is_stream", "capture_status",
	).Order("id DESC").Limit(pageSize).Offset(startIdx).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	for _, item := range items {
		item.IsFavorite = true
	}
	return items, total, nil
}

// GetFavoriteConversationContextByID returns one favorite owned by userID.
func GetFavoriteConversationContextByID(ctx context.Context, userID int, id int) (*FavoriteConversationContext, error) {
	var fav FavoriteConversationContext
	err := DB.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&fav).Error
	if err != nil {
		return nil, err
	}
	fav.IsFavorite = true
	return &fav, nil
}

// DeleteFavoriteConversationContext removes a favorite owned by userID.
func DeleteFavoriteConversationContext(ctx context.Context, userID int, id int) (int64, error) {
	result := DB.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&FavoriteConversationContext{})
	return result.RowsAffected, result.Error
}

func favoriteRequestIDSet(ctx context.Context, userID int, items []*ConversationContext) (map[string]bool, error) {
	requestIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item.RequestID != "" {
			requestIDs = append(requestIDs, item.RequestID)
		}
	}
	if len(requestIDs) == 0 {
		return map[string]bool{}, nil
	}
	var favs []FavoriteConversationContext
	if err := DB.WithContext(ctx).Select("request_id").
		Where("user_id = ? AND request_id IN ?", userID, requestIDs).Find(&favs).Error; err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(favs))
	for _, fav := range favs {
		set[fav.RequestID] = true
	}
	return set, nil
}

// markLogsHasContext batch-marks each log's HasContext by checking whether DB A
// holds a conversation context for its request_id. It uses one deduplicated IN
// query to avoid N+1 lookups and is strictly best-effort: a temporarily
// unavailable context database only leaves flags false (and is logged), so the
// usage log list keeps working even when DB A fails. Flags are always reset to
// false first — before any DB A access — so reused Log objects never carry
// stale values, even when DB A is nil or failing.
func markLogsHasContext(logs []*Log) {
	requestIDs := make([]string, 0, len(logs))
	seen := make(map[string]struct{}, len(logs))
	for _, log := range logs {
		if log == nil {
			continue
		}
		log.HasContext = false
		if log.RequestId == "" {
			continue
		}
		if _, dup := seen[log.RequestId]; dup {
			continue
		}
		seen[log.RequestId] = struct{}{}
		requestIDs = append(requestIDs, log.RequestId)
	}
	if CONVERSATION_DB == nil || len(requestIDs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var found []string
	err := CONVERSATION_DB.WithContext(ctx).Model(&ConversationContext{}).
		Where("request_id IN ?", requestIDs).
		Pluck("request_id", &found).Error
	if err != nil {
		common.SysLog("failed to mark log has_context: " + err.Error())
		return
	}
	set := make(map[string]bool, len(found))
	for _, requestID := range found {
		set[requestID] = true
	}
	for _, log := range logs {
		if log != nil {
			log.HasContext = set[log.RequestId]
		}
	}
}
