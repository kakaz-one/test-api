package task

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestServer はテスト用のハンドラを組み立てる。
// httptest.NewServer は使わず ServeMux に直接リクエストを流すので、
// 実際のポートを開かずにテストできる（速いし並列実行しても衝突しない）。
func newTestServer() http.Handler {
	mux := http.NewServeMux()
	NewHandler(NewStore()).Register(mux)
	return mux
}

// do はリクエストを 1 件実行してレスポンスを返すヘルパー。
func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}

	// ResponseRecorder はレスポンスを記録するだけの ResponseWriter 実装。
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decodeTask(t *testing.T, w *httptest.ResponseRecorder) Task {
	t.Helper()
	var got Task
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("レスポンスの JSON を読めませんでした: %v (body=%s)", err, w.Body.String())
	}
	return got
}

func TestCreateAndGetTask(t *testing.T) {
	h := newTestServer()

	w := do(t, h, http.MethodPost, "/tasks", `{"title":"Go を学ぶ"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	created := decodeTask(t, w)
	if created.Title != "Go を学ぶ" {
		t.Errorf("title = %q, want %q", created.Title, "Go を学ぶ")
	}
	if created.Done {
		t.Error("作成直後の done は false であるべき")
	}
	if got, want := w.Header().Get("Location"), "/tasks/1"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	w = do(t, h, http.MethodGet, "/tasks/1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := decodeTask(t, w); got.ID != created.ID {
		t.Errorf("id = %d, want %d", got.ID, created.ID)
	}
}

func TestListReturnsEmptyArray(t *testing.T) {
	h := newTestServer()

	w := do(t, h, http.MethodGet, "/tasks", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	// nil スライスを返すと "null" になってしまうので、"[]" になることを確認する。
	if got := strings.TrimSpace(w.Body.String()); got != "[]" {
		t.Errorf("body = %s, want []", got)
	}
}

func TestUpdateTask(t *testing.T) {
	h := newTestServer()
	do(t, h, http.MethodPost, "/tasks", `{"title":"買い物"}`)

	// done だけを更新する → title は変わらないはず。
	w := do(t, h, http.MethodPatch, "/tasks/1", `{"done":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	got := decodeTask(t, w)
	if !got.Done {
		t.Error("done = false, want true")
	}
	if got.Title != "買い物" {
		t.Errorf("title = %q, want %q（未指定のフィールドは変わらないはず）", got.Title, "買い物")
	}
}

func TestDeleteTask(t *testing.T) {
	h := newTestServer()
	do(t, h, http.MethodPost, "/tasks", `{"title":"削除される"}`)

	if w := do(t, h, http.MethodDelete, "/tasks/1", ""); w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if w := do(t, h, http.MethodGet, "/tasks/1", ""); w.Code != http.StatusNotFound {
		t.Fatalf("削除後の GET: status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// テーブルドリブンテスト: 入力と期待値を表にして 1 つのループで回す、Go で定番の書き方。
func TestErrorResponses(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"タイトルが空", http.MethodPost, "/tasks", `{"title":"   "}`, http.StatusBadRequest},
		{"JSON が壊れている", http.MethodPost, "/tasks", `{"title":`, http.StatusBadRequest},
		{"知らないキー", http.MethodPost, "/tasks", `{"titel":"typo"}`, http.StatusBadRequest},
		{"ボディが空", http.MethodPost, "/tasks", "", http.StatusBadRequest},
		{"id が数値でない", http.MethodGet, "/tasks/abc", "", http.StatusBadRequest},
		{"存在しない id", http.MethodGet, "/tasks/999", "", http.StatusNotFound},
		{"更新対象が無い", http.MethodPatch, "/tasks/999", `{"done":true}`, http.StatusNotFound},
		{"更新項目が空", http.MethodPatch, "/tasks/1", `{}`, http.StatusBadRequest},
		{"許可されていないメソッド", http.MethodPut, "/tasks/1", `{}`, http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestServer()
			do(t, h, http.MethodPost, "/tasks", `{"title":"下準備"}`)

			w := do(t, h, tt.method, tt.path, tt.body)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body=%s)", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}
