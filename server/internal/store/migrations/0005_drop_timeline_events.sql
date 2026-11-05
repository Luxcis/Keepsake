-- 时光轴不再维护手工事件，改由已发布旅行自动生成；删除旧事件表及其索引。
DROP TABLE IF EXISTS timeline_events;
