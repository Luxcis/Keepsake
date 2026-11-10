package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func newTripTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "journey.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}

func createTestTrip(t *testing.T, st *Store, title, startDate, status string) string {
	t.Helper()
	id, err := st.CreateTrip(context.Background(), TripInput{
		Title:     title,
		StartDate: startDate,
		Status:    status,
	})
	if err != nil {
		t.Fatalf("create trip %s: %v", title, err)
	}
	return id
}

func pinnedByID(t *testing.T, st *Store) map[string]bool {
	t.Helper()
	trips, err := st.ListTripsAdmin(context.Background())
	if err != nil {
		t.Fatalf("list admin trips: %v", err)
	}
	out := make(map[string]bool, len(trips))
	for _, trip := range trips {
		out[trip.ID] = trip.Pinned
	}
	return out
}

func countPinned(t *testing.T, st *Store) int {
	t.Helper()
	var n int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM trips WHERE is_pinned = 1`).Scan(&n); err != nil {
		t.Fatalf("count pinned: %v", err)
	}
	return n
}

// 同一时刻最多一个置顶旅行：置顶新旅行会取消原置顶，取消后不再有置顶。
func TestSetTripPinnedKeepsSinglePin(t *testing.T) {
	ctx := context.Background()
	st := newTripTestStore(t)
	a := createTestTrip(t, st, "A", "2024-01-01", "published")
	b := createTestTrip(t, st, "B", "2024-02-01", "published")

	if err := st.SetTripPinned(ctx, a, true); err != nil {
		t.Fatalf("pin A: %v", err)
	}
	if got := pinnedByID(t, st); !got[a] || got[b] {
		t.Fatalf("after pin A: %v", got)
	}
	if got := countPinned(t, st); got != 1 {
		t.Fatalf("pinned rows = %d, want 1", got)
	}

	if err := st.SetTripPinned(ctx, b, true); err != nil {
		t.Fatalf("pin B: %v", err)
	}
	if got := pinnedByID(t, st); got[a] || !got[b] {
		t.Fatalf("after pin B: %v", got)
	}
	if got := countPinned(t, st); got != 1 {
		t.Fatalf("pinned rows = %d, want 1", got)
	}

	if err := st.SetTripPinned(ctx, b, false); err != nil {
		t.Fatalf("unpin B: %v", err)
	}
	if got := countPinned(t, st); got != 0 {
		t.Fatalf("pinned rows = %d, want 0", got)
	}
}

// 不存在的旅行返回 ErrNotFound，且不会清除已有置顶。
func TestSetTripPinnedMissingTripKeepsExistingPin(t *testing.T) {
	ctx := context.Background()
	st := newTripTestStore(t)
	a := createTestTrip(t, st, "A", "2024-01-01", "published")

	if err := st.SetTripPinned(ctx, a, true); err != nil {
		t.Fatalf("pin A: %v", err)
	}
	if err := st.SetTripPinned(ctx, "missing", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pin missing = %v, want ErrNotFound", err)
	}
	if got := pinnedByID(t, st); !got[a] {
		t.Fatalf("existing pin cleared: %v", got)
	}
}

// 草稿可以置顶，但不会出现在公开列表；发布后公开列表带 pinned 标记。
func TestTripPinnedDraftNotPublic(t *testing.T) {
	ctx := context.Background()
	st := newTripTestStore(t)
	draft := createTestTrip(t, st, "Draft", "2024-03-01", "draft")

	if err := st.SetTripPinned(ctx, draft, true); err != nil {
		t.Fatalf("pin draft: %v", err)
	}
	public, err := st.ListTrips(ctx)
	if err != nil {
		t.Fatalf("list public trips: %v", err)
	}
	if len(public) != 0 {
		t.Fatalf("public trips = %d, want 0", len(public))
	}

	// 发布后置顶标记随公开列表返回。
	if err := st.UpdateTrip(ctx, draft, TripInput{
		Title: "Draft", StartDate: "2024-03-01", Status: "published",
	}); err != nil {
		t.Fatalf("publish draft: %v", err)
	}
	public, err = st.ListTrips(ctx)
	if err != nil {
		t.Fatalf("list public trips: %v", err)
	}
	if len(public) != 1 || !public[0].Pinned {
		t.Fatalf("public trips = %+v, want one pinned trip", public)
	}
}

// Trip 的自定义 JSON 序列化必须携带置顶标记：前台按该字段拆分展示。
func TestTripJSONIncludesPinned(t *testing.T) {
	raw, err := json.Marshal(Trip{ID: "t1", Title: "A", Pinned: true})
	if err != nil {
		t.Fatalf("marshal trip: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal trip: %v", err)
	}
	if v, ok := got["pinned"]; !ok || v != true {
		t.Fatalf("pinned = %v (present=%v), want true", v, ok)
	}
}

// 删除置顶旅行后不再有置顶记录，其他旅行仍可置顶。
func TestDeletePinnedTripReleasesPin(t *testing.T) {
	ctx := context.Background()
	st := newTripTestStore(t)
	a := createTestTrip(t, st, "A", "2024-01-01", "published")
	b := createTestTrip(t, st, "B", "2024-02-01", "published")

	if err := st.SetTripPinned(ctx, a, true); err != nil {
		t.Fatalf("pin A: %v", err)
	}
	if err := st.DeleteTrip(ctx, a); err != nil {
		t.Fatalf("delete A: %v", err)
	}
	if got := countPinned(t, st); got != 0 {
		t.Fatalf("pinned rows = %d, want 0", got)
	}
	if err := st.SetTripPinned(ctx, b, true); err != nil {
		t.Fatalf("pin B after delete: %v", err)
	}
	if got := pinnedByID(t, st); !got[b] {
		t.Fatalf("pin B not applied: %v", got)
	}
}
