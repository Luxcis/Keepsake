-- 0006_photo_media_kind.sql —— 独立视频条目支持：
-- media_kind 区分照片（photo）与单独上传的视频（video）。
-- 视频记录的 cos_key 直接指向视频对象，video_key 仅用于为照片关联实况视频。
-- 历史记录一律视为照片，与改动前行为一致。

ALTER TABLE photos ADD COLUMN media_kind TEXT NOT NULL DEFAULT 'photo';
