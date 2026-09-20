package task

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
)

// Handler は Store を HTTP API として公開する。
type Handler struct {
	store *Store
}

// NewHandler は Handler を作る。
func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// Register はルーティングを mux に登録する。
// Go 1.22 以降の http.ServeMux は "METHOD /path/{param}" 形式のパターンを
// 書けるようになったので、外部ルーターを入れなくてもこの程度の API は組める。
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /tasks", h.list)
	mux.HandleFunc("POST /tasks", h.create)
	mux.HandleFunc("GET /tasks/{id}", h.get)
	mux.HandleFunc("PATCH /tasks/{id}", h.update)
	mux.HandleFunc("DELETE /tasks/{id}", h.delete)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) list(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.store.List())
}

// createRequest は POST /tasks のリクエストボディ。
type createRequest struct {
	Title string `json:"title"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	t, err := h.store.Create(req.Title)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	// 201 Created では作成したリソースの場所を Location ヘッダで示すのが作法。
	w.Header().Set("Location", "/tasks/"+strconv.Itoa(t.ID))
	writeJSON(w, http.StatusCreated, t)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	t, err := h.store.Get(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// updateRequest は PATCH /tasks/{id} のリクエストボディ。
// ポインタにしておくと「キーが無かった」と「明示的に false / 空文字が来た」を
// 区別できる（nil なら未指定）。部分更新ではこの区別が重要。
type updateRequest struct {
	Title *string `json:"title"`
	Done  *bool   `json:"done"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var req updateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Title == nil && req.Done == nil {
		writeError(w, http.StatusBadRequest, "title または done のどちらかを指定してください")
		return
	}

	t, err := h.store.Update(id, req.Title, req.Done)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	if err := h.store.Delete(id); err != nil {
		writeStoreError(w, err)
		return
	}
	// 削除成功はボディ無しの 204 を返す。
	w.WriteHeader(http.StatusNoContent)
}

// pathID は URL の {id} を数値として取り出す。
// 取り出せなかった場合はここで 400 を返し、false を返す。
func pathID(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.PathValue("id")
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id は正の整数で指定してください")
		return 0, false
	}
	return id, true
}

// decodeJSON はリクエストボディを構造体に読み込む。
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	// MaxBytesReader で読み込み量に上限を付ける。
	// 上限が無いと巨大なボディを送りつけられただけでメモリを食い潰せてしまう。
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)) // 1MB 上限
	// 知らないキーが来たらエラーにする。タイポを早期に気づけるので学習用途では便利。
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("リクエストボディが空です")
		}
		return errors.New("JSON の形式が不正です: " + err.Error())
	}
	// ボディに JSON が 2 個以上続いていないか確認する。
	if dec.More() {
		return errors.New("リクエストボディに余分なデータがあります")
	}
	return nil
}

// writeJSON はステータスコードと JSON ボディを書き出す。
func writeJSON(w http.ResponseWriter, status int, body any) {
	// Content-Type は WriteHeader より前に設定しないと反映されない。
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// ここまで来るとヘッダは送信済みなので、ログに残すことしかできない。
		slog.Error("failed to encode response", "error", err)
	}
}

// writeError はエラーレスポンスの形式を 1 か所に揃えるためのヘルパー。
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeStoreError は Store が返したエラーを HTTP ステータスへ翻訳する。
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "指定されたタスクは存在しません")
	case errors.Is(err, ErrInvalidTitle):
		writeError(w, http.StatusBadRequest, "title は 1〜200 文字で指定してください")
	default:
		slog.Error("unexpected store error", "error", err)
		writeError(w, http.StatusInternalServerError, "内部エラーが発生しました")
	}
}
