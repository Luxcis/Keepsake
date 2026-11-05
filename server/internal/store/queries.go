package store

import (
	"context"
	"database/sql"
)

// 只读骨架 API 的数据访问：脚手架阶段先提供旅行/时光轴/站点设定的真实查询，
// 相册、照片签名 URL、生日信与门禁在后续阶段接入（见 README 阶段路线）。
// 约定（计划 §3.2）：存储层一律 UTC；展示层负责时区格式化。

type Trip struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Summary      string         `json:"summary"`
	StartDate    string         `json:"start_date"`
	EndDate      sql.NullString `json:"end_date"`
	CoverPhotoID sql.NullString `json:"cover_photo_id"`
	// 瞬态字段：handlers 层按 CoverPhotoID 签名的封面缩略图 URL（store 不写，见 marshal.go）。
	CoverThumbSrc string `json:"cover_thumb_src"`
	SortOrder     int    `json:"sort_order"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type TripLocation struct {
	ID        string          `json:"id"`
	TripID    string          `json:"trip_id"`
	Name      string          `json:"name"`
	Address   sql.NullString  `json:"address"`
	Latitude  sql.NullFloat64 `json:"latitude"`
	Longitude sql.NullFloat64 `json:"longitude"`
	Note      sql.NullString  `json:"note"`
	SortOrder int             `json:"sort_order"`
}

type TimelineEvent struct {
	ID          string         `json:"id"`
	EventDate   string         `json:"event_date"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	PhotoID     sql.NullString `json:"photo_id"`
	SortOrder   int            `json:"sort_order"`
}

// ListTrips 返回已发布的旅行列表（公开接口）。
func (s *Store) ListTrips(ctx context.Context) ([]Trip, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, title, summary, start_date, end_date, cover_photo_id, sort_order, status, created_at, updated_at
		FROM trips
		WHERE status = 'published'
		ORDER BY sort_order ASC, start_date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trips := []Trip{}
	for rows.Next() {
		var t Trip
		if err := rows.Scan(&t.ID, &t.Title, &t.Summary, &t.StartDate, &t.EndDate,
			&t.CoverPhotoID, &t.SortOrder, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		trips = append(trips, t)
	}
	return trips, rows.Err()
}

// GetTrip 返回单次旅行与关联地点；仅允许已发布状态（公开接口）。
func (s *Store) GetTrip(ctx context.Context, id string) (*Trip, []TripLocation, error) {
	var t Trip
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, title, summary, start_date, end_date, cover_photo_id, sort_order, status, created_at, updated_at
		FROM trips WHERE id = ?`, id).
		Scan(&t.ID, &t.Title, &t.Summary, &t.StartDate, &t.EndDate,
			&t.CoverPhotoID, &t.SortOrder, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows || (err == nil && t.Status != "published") {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, trip_id, name, address, latitude, longitude, note, sort_order
		FROM trip_locations WHERE trip_id = ? ORDER BY sort_order ASC`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	locations := []TripLocation{}
	for rows.Next() {
		var l TripLocation
		if err := rows.Scan(&l.ID, &l.TripID, &l.Name, &l.Address,
			&l.Latitude, &l.Longitude, &l.Note, &l.SortOrder); err != nil {
			return nil, nil, err
		}
		locations = append(locations, l)
	}
	return &t, locations, rows.Err()
}

// ListTimeline 返回全部时光轴事件，按日期升序（展示层可再倒序）。
func (s *Store) ListTimeline(ctx context.Context) ([]TimelineEvent, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, event_date, title, description, photo_id, sort_order
		FROM timeline_events ORDER BY event_date ASC, sort_order ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []TimelineEvent{}
	for rows.Next() {
		var e TimelineEvent
		if err := rows.Scan(&e.ID, &e.EventDate, &e.Title, &e.Description,
			&e.PhotoID, &e.SortOrder); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// SettingsWhitelist：站点配置中允许公开返回的字段（计划 §4.2）。
var SettingsWhitelist = []string{
	"site_title", "hero_kicker", "hero_title", "hero_subtitle", "hero_cta", "site_motto",
}

// GetSiteSettings 读取白名单内的站点配置；不存在的 key 返回空字符串，
// 白名单键固定（当前 6 个），无需一次 IN 查询，逐键读取足够简单。
func (s *Store) GetSiteSettings(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(SettingsWhitelist))
	for _, key := range SettingsWhitelist {
		out[key] = "" // 保证白名单字段全部出现
	}
	for _, key := range SettingsWhitelist {
		var value string
		qerr := s.DB.QueryRowContext(ctx,
			`SELECT value FROM site_settings WHERE key = ?`, key).Scan(&value)
		if qerr == nil {
			out[key] = value
		} else if qerr != sql.ErrNoRows {
			return nil, qerr
		}
	}
	return out, nil
}
