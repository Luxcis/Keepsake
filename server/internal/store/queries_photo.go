package store

import (
	"context"
	"database/sql"
	"time"
)

// 照片数据访问（计划 §4.1 photos / §4.3 上传链路）。
// 只保存 COS Key 与元数据，不存二进制；URL 由 handlers 层签名。

type Photo struct {
	ID           string         `json:"id"`
	TripID       string         `json:"trip_id"`
	AlbumID      sql.NullString `json:"album_id"`
	COSKey       string         `json:"cos_key"`
	ThumbnailKey sql.NullString `json:"thumbnail_key"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	TakenAt      sql.NullString `json:"taken_at"`
	Width        sql.NullInt64  `json:"width"`
	Height       sql.NullInt64  `json:"height"`
	SortOrder    int            `json:"sort_order"`
	CreatedAt    string         `json:"created_at"`
}

type PhotoInput struct {
	TripID      string  `json:"trip_id"`
	AlbumID     *string `json:"album_id"`
	COSKey      string
	Title       string  `json:"title"`
	Description string  `json:"description"`
	TakenAt     *string `json:"taken_at"`
	Width       *int64  `json:"width"`
	Height      *int64  `json:"height"`
}

func photoRowID() string { return newID() }

func (s *Store) CreatePhoto(ctx context.Context, in PhotoInput) (string, error) {
	var album any
	if in.AlbumID != nil && *in.AlbumID != "" {
		album = *in.AlbumID
	}
	var taken any
	if in.TakenAt != nil && *in.TakenAt != "" {
		taken = *in.TakenAt
	}
	id := photoRowID()
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO photos (id, trip_id, album_id, cos_key, thumbnail_key, title, description, taken_at, width, height, sort_order, created_at)
		VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.TripID, album, in.COSKey, in.Title, in.Description, taken,
		nullInt(in.Width), nullInt(in.Height), 9999, nowUTC())
	return id, err
}

func nullInt(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// AlbumRef 控制照片归属更新：Set==nil 保持不变；Set!=nil 按指向调整（空串=挪回旅行层级）。
type AlbumRef struct {
	Set *string
}

func (s *Store) UpdatePhotoMeta(ctx context.Context, id string, in PhotoInput, album AlbumRef) error {
	var source Photo
	err := s.DB.QueryRowContext(ctx, `
		SELECT album_id FROM photos WHERE id = ?`, id).Scan(&source.AlbumID)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	var albumVal any
	if album.Set != nil {
		if *album.Set == "" {
			albumVal = nil
		} else {
			albumVal = *album.Set
		}
	} else {
		albumVal = source.AlbumID // 保持不变（Valid=false 时即为 NULL）
	}

	var taken any
	if in.TakenAt != nil && *in.TakenAt != "" {
		taken = *in.TakenAt
	} else {
		taken = nil
	}
	_, err = s.DB.ExecContext(ctx, `
		UPDATE photos SET title=?, description=?, album_id=?, taken_at=? WHERE id=?`,
		in.Title, in.Description, albumVal, taken, id)
	return err
}

func (s *Store) ListPhotosByTrip(ctx context.Context, tripID string) ([]Photo, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, trip_id, album_id, cos_key, thumbnail_key, title, description, taken_at, width, height, sort_order, created_at
		FROM photos WHERE trip_id = ? ORDER BY sort_order ASC, created_at ASC`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Photo{}
	for rows.Next() {
		var p Photo
		if err := rows.Scan(&p.ID, &p.TripID, &p.AlbumID, &p.COSKey, &p.ThumbnailKey,
			&p.Title, &p.Description, &p.TakenAt, &p.Width, &p.Height, &p.SortOrder, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPhotoKeys 查询单张照片的归属与对象 Key，供管理端封面校验与公开封面签名共用。
func (s *Store) GetPhotoKeys(ctx context.Context, id string) (tripID, cosKey string, thumbKey sql.NullString, err error) {
	err = s.DB.QueryRowContext(ctx,
		`SELECT trip_id, cos_key, thumbnail_key FROM photos WHERE id = ?`, id).
		Scan(&tripID, &cosKey, &thumbKey)
	if err == sql.ErrNoRows {
		return "", "", sql.NullString{}, ErrNotFound
	}
	if err != nil {
		return "", "", sql.NullString{}, err
	}
	return tripID, cosKey, thumbKey, nil
}

// DeletePhoto 删除数据库关联并返回 COS Key（原图+缩略图）供调用方补偿清理（计划 §4.1）。
// cover_photo_id 无外键（与 photos 循环引用），删除照片时在此归空相关旅行封面。
func (s *Store) DeletePhoto(ctx context.Context, id string) (cosKey, thumbKey string, err error) {
	var thumb sql.NullString
	err = s.DB.QueryRowContext(ctx, `SELECT cos_key, thumbnail_key FROM photos WHERE id = ?`, id).
		Scan(&cosKey, &thumb)
	if err == sql.ErrNoRows {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM photos WHERE id = ?`, id); err != nil {
		return "", "", err
	}
	if _, err := s.DB.ExecContext(ctx,
		`UPDATE trips SET cover_photo_id = NULL WHERE cover_photo_id = ?`, id); err != nil {
		return "", "", err
	}
	return cosKey, thumb.String, nil
}

// GetPhoto 校验存在性（确认阶段避免重复 Key：由 UNIQUE 约束兜底）。
func (s *Store) GetPhotoID(ctx context.Context, id string) (string, error) {
	var got string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM photos WHERE id = ?`, id).Scan(&got)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return got, err
}

// ReorderPhotos 按传入顺序重排（short tx；下标即 sort_order）。
// 未包含在 ids 中的照片不移动（调用方保证全量或子集一致）。
func (s *Store) ReorderPhotos(ctx context.Context, ids []string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE photos SET sort_order = ? WHERE id = ?`, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- COS 删除补偿任务（0003） ----------

type DeleteTask struct {
	ID     string `json:"id"`
	COSKey string `json:"cos_key"`
}

func (s *Store) EnqueueDeleteTask(ctx context.Context, cosKey string) (string, error) {
	id := newID()
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO cos_delete_tasks (id, cos_key, created_at) VALUES (?, ?, ?)`,
		id, cosKey, nowUTC())
	return id, err
}

func (s *Store) PendingDeleteTasks(ctx context.Context, limit int) ([]DeleteTask, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, cos_key, attempts FROM cos_delete_tasks
		WHERE done_at IS NULL AND attempts < 20
		ORDER BY created_at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeleteTask{}
	for rows.Next() {
		var t DeleteTask
		if err := rows.Scan(&t.ID, &t.COSKey); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RecordTaskFailure(ctx context.Context, id, message string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE cos_delete_tasks
		SET attempts = attempts + 1, last_error = ?
		WHERE id = ?`, message, id)
	return err
}

func (s *Store) CompleteDeleteTask(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE cos_delete_tasks SET done_at = ?, last_error = NULL WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id)
	return err
}
