package store

import (
	"database/sql"
	"encoding/json"
)

// 自定义 JSON 序列化：sql.Null* 类型的默认编码会泄漏 {String, Valid} 内部结构，
// 这里统一输出干净的 string|null / number|null（API 契约见计划 §4.2）。

// helper: valueOrNull —— valid 时返回 v 否则 nil。
func valueOrNull[T any](valid bool, v T) any {
	if !valid {
		return nil
	}
	return v
}

func nullStr(ns sql.NullString) any { return valueOrNull(ns.Valid, ns.String) }

func nullNum(ni sql.NullInt64) any { return valueOrNull(ni.Valid, ni.Int64) }

func nullF64(nf sql.NullFloat64) any { return valueOrNull(nf.Valid, nf.Float64) }

func (t Trip) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":              t.ID,
		"title":           t.Title,
		"summary":         t.Summary,
		"start_date":      t.StartDate,
		"end_date":        nullStr(t.EndDate),
		"cover_photo_id":  nullStr(t.CoverPhotoID),
		"cover_thumb_src": t.CoverThumbSrc,
		"sort_order":      t.SortOrder,
		"status":          t.Status,
		"created_at":      t.CreatedAt,
		"updated_at":      t.UpdatedAt,
	})
}

func (l TripLocation) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":         l.ID,
		"trip_id":    l.TripID,
		"name":       l.Name,
		"address":    nullStr(l.Address),
		"latitude":   nullF64(l.Latitude),
		"longitude":  nullF64(l.Longitude),
		"note":       nullStr(l.Note),
		"sort_order": l.SortOrder,
	})
}

func (e TimelineEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":          e.ID,
		"event_date":  e.EventDate,
		"title":       e.Title,
		"description": e.Description,
		"photo_id":    nullStr(e.PhotoID),
		"sort_order":  e.SortOrder,
	})
}

func (a Album) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":          a.ID,
		"trip_id":     a.TripID,
		"title":       a.Title,
		"description": a.Description,
		"sort_order":  a.SortOrder,
	})
}
