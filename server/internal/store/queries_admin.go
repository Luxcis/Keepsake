package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 管理端数据访问（计划 §4.2 写接口）。
// 约定：短事务（计划 §3.2）；主键由服务端 newID 生成；时间一律 UTC RFC3339。

var ErrNoUser = errors.New("user not found")

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// ---------- 管理员账号（users 表，计划 §4.1） ----------

func (s *Store) CreateAdmin(ctx context.Context, username, passwordHash string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, created_at)
		VALUES (?, ?, ?, 'admin', ?)`, newID(), username, passwordHash, nowUTC())
	if isUniqueViolation(err) {
		return fmt.Errorf("username %q already exists: %w", username, err)
	}
	return err
}

func (s *Store) AdminPasswordHash(ctx context.Context, username string) (string, error) {
	var hash string
	err := s.DB.QueryRowContext(ctx,
		`SELECT password_hash FROM users WHERE username = ?`, username).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", ErrNoUser
	}
	return hash, err
}

func (s *Store) AdminRole(ctx context.Context, username string) (string, error) {
	var role string
	err := s.DB.QueryRowContext(ctx, `SELECT role FROM users WHERE username = ?`, username).Scan(&role)
	if err == sql.ErrNoRows {
		return "", ErrNoUser
	}
	return role, err
}

// isUniqueViolation：modernc 驱动返回错误文本中包含 SQLite 约束失败信息的识别。
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "constraint failed")
}

// ---------- 站点设置 ----------

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	if err := s.DB.QueryRowContext(ctx,
		`SELECT value FROM site_settings WHERE key = ?`, key).Scan(&value); err == sql.ErrNoRows {
		return "", nil
	} else if err != nil {
		return "", err
	}
	return value, nil
}

// SetSettings 批量 upsert（调用方负责白名单校验）。
func (s *Store) SetSettings(ctx context.Context, kv map[string]string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO site_settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	ts := nowUTC()
	for k, v := range kv {
		if _, err := stmt.ExecContext(ctx, k, v, ts); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- 信件（单实例：id 固定 "letter"，计划 §4.1 letters） ----------

type Letter struct {
	Title         string `json:"title"`
	Body          string `json:"body"`
	AudioKey      string `json:"audio_key"`
	RevealEnabled bool   `json:"reveal_enabled"`
	UpdatedAt     string `json:"updated_at"`
}

const letterRowID = "letter"

func (s *Store) GetLetter(ctx context.Context) (*Letter, error) {
	var l Letter
	var audio sql.NullString
	var reveal int
	err := s.DB.QueryRowContext(ctx, `
		SELECT title, body, audio_key, reveal_enabled, updated_at
		FROM letters WHERE id = ?`, letterRowID).
		Scan(&l.Title, &l.Body, &audio, &reveal, &l.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	l.AudioKey = audio.String
	l.RevealEnabled = reveal == 1
	return &l, nil
}

func (s *Store) UpsertLetter(ctx context.Context, l Letter) error {
	reveal := 0
	if l.RevealEnabled {
		reveal = 1
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO letters (id, title, body, audio_key, reveal_enabled, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title, body = excluded.body, audio_key = excluded.audio_key,
			reveal_enabled = excluded.reveal_enabled, updated_at = excluded.updated_at`,
		letterRowID, l.Title, l.Body, nullString(l.AudioKey), reveal, nowUTC())
	return err
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ---------- 旅行 CRUD（trips / trip_locations / albums） ----------

type TripInput struct {
	Title     string  `json:"title"`
	Summary   string  `json:"summary"`
	StartDate string  `json:"start_date"`
	EndDate   *string `json:"end_date"`
	// CoverPhotoID：缺失/null = 保持不变（兼容 toggleStatus 等不带该字段的调用）；
	// "" = 清除；非空 = 设置（管理端校验照片存在且属于该旅行）。
	CoverPhotoID *string `json:"cover_photo_id"`
	SortOrder    int     `json:"sort_order"`
	Status       string  `json:"status"`
}

func (s *Store) CreateTrip(ctx context.Context, in TripInput) (string, error) {
	id := newID()
	ts := nowUTC()
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO trips (id, title, summary, start_date, end_date, cover_photo_id, sort_order, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NULL, ?, ?, ?, ?)`,
		id, in.Title, in.Summary, in.StartDate, nullPtrStr(in.EndDate), in.SortOrder, in.Status, ts, ts)
	if err != nil {
		return "", err
	}
	return id, nil
}

func nullPtrStr(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return *p
}

func (s *Store) UpdateTrip(ctx context.Context, id string, in TripInput) error {
	// cover_photo_id 只在显式传入（指针非 nil）时写入；否则保持现状，方便既有调用方。
	set := "title = ?, summary = ?, start_date = ?, end_date = ?, sort_order = ?, status = ?, updated_at = ?"
	args := []any{in.Title, in.Summary, in.StartDate, nullPtrStr(in.EndDate),
		in.SortOrder, in.Status, nowUTC()}
	if in.CoverPhotoID != nil {
		set += ", cover_photo_id = ?"
		args = append(args, nullPtrStr(in.CoverPhotoID))
	}
	args = append(args, id)

	res, err := s.DB.ExecContext(ctx, `
		UPDATE trips SET `+set+`
		WHERE id = ?`, args...)
	if err != nil {
		return err
	}
	return requireRowsAffected(res, id)
}

func (s *Store) DeleteTrip(ctx context.Context, id string) error {
	// 依赖外键级联：trip_locations/albums 级联删除；photos 级联删除（数据库关联先行，
	// COS 对象的补偿删除由 COS 阶段的任务化清理实现，计划 §4.1）。
	res, err := s.DB.ExecContext(ctx, `DELETE FROM trips WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireRowsAffected(res, id)
}

// ListTripsAdmin 返回全部旅行（含草稿），管理端使用。
func (s *Store) ListTripsAdmin(ctx context.Context) ([]Trip, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, title, summary, start_date, end_date, cover_photo_id, sort_order, status, created_at, updated_at
		FROM trips ORDER BY sort_order ASC, start_date DESC`)
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

func requireRowsAffected(res sql.Result, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetTripAny 与 GetTrip 相同，但忽略发布状态（管理端编辑草稿使用）。
func (s *Store) GetTripAny(ctx context.Context, id string) (*Trip, []TripLocation, error) {
	var t Trip
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, title, summary, start_date, end_date, cover_photo_id, sort_order, status, created_at, updated_at
		FROM trips WHERE id = ?`, id).
		Scan(&t.ID, &t.Title, &t.Summary, &t.StartDate, &t.EndDate,
			&t.CoverPhotoID, &t.SortOrder, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
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

// TripLocationInput 是管理端写入的地点入参（JSON 友好；可选字段用指针表达 null）。
type TripLocationInput struct {
	Name      string   `json:"name"`
	Address   *string  `json:"address"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Note      *string  `json:"note"`
}

func toLocationRow(tripID string, in TripLocationInput, order int) TripLocation {
	var addr, note sql.NullString
	if in.Address != nil {
		addr = sql.NullString{String: *in.Address, Valid: true}
	}
	if in.Note != nil {
		note = sql.NullString{String: *in.Note, Valid: true}
	}
	var lat, lng sql.NullFloat64
	if in.Latitude != nil {
		lat = sql.NullFloat64{Float64: *in.Latitude, Valid: true}
	}
	if in.Longitude != nil {
		lng = sql.NullFloat64{Float64: *in.Longitude, Valid: true}
	}
	return TripLocation{TripID: tripID, Name: in.Name, Address: addr,
		Latitude: lat, Longitude: lng, Note: note, SortOrder: order}
}

// ReplaceTripLocations 以整组替换策略维护地点及排序（短事务：数据量小、单次提交）。
func (s *Store) ReplaceTripLocations(ctx context.Context, tripID string, locs []TripLocationInput) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.ExecContext(ctx, `DELETE FROM trip_locations WHERE trip_id = ?`, tripID); err != nil {
		return err
	}
	for i, in := range locs {
		l := toLocationRow(tripID, in, i)
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO trip_locations (id, trip_id, name, address, latitude, longitude, note, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			newID(), l.TripID, l.Name, l.Address, l.Latitude, l.Longitude, l.Note, l.SortOrder); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- 相册 ----------

func (s *Store) ListAlbums(ctx context.Context, tripID string) ([]Album, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, trip_id, title, description, sort_order FROM albums
		WHERE trip_id = ? ORDER BY sort_order ASC`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Album{}
	for rows.Next() {
		var a Album
		if err := rows.Scan(&a.ID, &a.TripID, &a.Title, &a.Description, &a.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type Album struct {
	ID          string `json:"id"`
	TripID      string `json:"trip_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

type AlbumInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

func (s *Store) CreateAlbum(ctx context.Context, tripID string, in AlbumInput) (string, error) {
	id := newID()
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO albums (id, trip_id, title, description, sort_order) VALUES (?, ?, ?, ?, ?)`,
		id, tripID, in.Title, in.Description, in.SortOrder)
	return id, err
}

func (s *Store) UpdateAlbum(ctx context.Context, id string, in AlbumInput) error {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE albums SET title=?, description=?, sort_order=? WHERE id=?`,
		in.Title, in.Description, in.SortOrder, id)
	if err != nil {
		return err
	}
	return requireRowsAffected(res, id)
}

func (s *Store) DeleteAlbum(ctx context.Context, id string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM albums WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireRowsAffected(res, id)
}
