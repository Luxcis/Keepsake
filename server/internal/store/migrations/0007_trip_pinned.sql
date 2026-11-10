-- 0007_trip_pinned.sql —— 旅行置顶：
-- trips 增加置顶标记；同一时刻最多一个置顶旅行由局部唯一索引保证。
-- 历史记录默认为未置顶，与改动前行为一致。

ALTER TABLE trips ADD COLUMN is_pinned INTEGER NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX idx_trips_pinned ON trips(is_pinned) WHERE is_pinned = 1;
