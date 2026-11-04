-- 0003_cos_delete_tasks.sql —— 照片删除补偿任务（计划 §4.1：
-- “先删数据库关联、再异步/重试删除 COS 对象”，记录失败任务以便补偿）。

CREATE TABLE cos_delete_tasks (
    id         TEXT PRIMARY KEY,
    cos_key    TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    attempts   INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    done_at    TEXT
);
