-- 0001_init.sql —— 计划 §4.1 核心数据表。
-- 约定：主键 TEXT UUID/ULID；时间戳 TEXT(RFC3339, UTC)；日期 TEXT(YYYY-MM-DD)。
-- 应用层记录 UTC（计划 §3.2），展示层按 Asia/Shanghai 格式化。

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,                -- Argon2id（认证阶段实现）
    role          TEXT NOT NULL DEFAULT 'admin',
    created_at    TEXT NOT NULL
);

CREATE TABLE trips (
    id             TEXT PRIMARY KEY,
    title          TEXT NOT NULL,
    summary        TEXT NOT NULL DEFAULT '',
    start_date     TEXT NOT NULL,               -- YYYY-MM-DD
    end_date       TEXT,
    cover_photo_id TEXT,                        -- photos.id；与 photos 循环引用故不设外键，应用层保证
    sort_order     INTEGER NOT NULL DEFAULT 0,
    status         TEXT NOT NULL DEFAULT 'draft',  -- draft / published
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE TABLE trip_locations (
    id        TEXT PRIMARY KEY,
    trip_id   TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    address   TEXT,
    latitude  REAL,
    longitude REAL,
    note      TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE albums (
    id          TEXT PRIMARY KEY,
    trip_id     TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE photos (
    id            TEXT PRIMARY KEY,
    trip_id       TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    album_id      TEXT REFERENCES albums(id) ON DELETE SET NULL,
    cos_key       TEXT NOT NULL UNIQUE,          -- 服务端生成，禁止使用用户原始文件名（计划 §4.3）
    thumbnail_key TEXT,
    title         TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    taken_at      TEXT,                          -- 用户声明的拍摄时间（RFC3339）
    width         INTEGER,
    height        INTEGER,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL
);

CREATE TABLE timeline_events (
    id          TEXT PRIMARY KEY,
    event_date  TEXT NOT NULL,                  -- YYYY-MM-DD
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    photo_id    TEXT REFERENCES photos(id) ON DELETE SET NULL,
    sort_order  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE letters (
    id             TEXT PRIMARY KEY,
    title          TEXT NOT NULL,
    body           TEXT NOT NULL,
    audio_key      TEXT,                        -- COS 对象 Key，语音可选
    reveal_enabled INTEGER NOT NULL DEFAULT 1,  -- 是否允许“揭晓”交互
    updated_at     TEXT NOT NULL
);

CREATE TABLE site_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- 计划 §4.1：为 trip_id、album_id、taken_at、event_date、sort_order 建索引。
CREATE INDEX idx_trips_status_sort ON trips(status, sort_order, start_date DESC);
CREATE INDEX idx_trip_locations_trip ON trip_locations(trip_id, sort_order);
CREATE INDEX idx_albums_trip ON albums(trip_id, sort_order);
CREATE INDEX idx_photos_trip ON photos(trip_id);
CREATE INDEX idx_photos_album ON photos(album_id);
CREATE INDEX idx_photos_taken_at ON photos(taken_at);
CREATE INDEX idx_timeline_event_date ON timeline_events(event_date, sort_order);

-- 初始站点设置（占位文案，管理后台上线后可编辑）。
INSERT INTO site_settings (key, value, updated_at) VALUES
    ('site_title', '我们的旅行纪念册', strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    ('hero_kicker', 'TO MY FAVORITE PERSON · 2026.11.22', strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    ('hero_title', '和你走过的每一站。', strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    ('hero_subtitle', '有些风景值得收藏，有些瞬间，因为有你才变得特别。这一次，想邀请你重新走一遍我们的故事。', strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    ('hero_cta', '开启我们的故事', strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    ('site_motto', 'A JOURNEY THROUGH US', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'));
