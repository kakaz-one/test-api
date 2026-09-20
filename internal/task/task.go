// Package task は TODO タスクのドメインモデルと保存・HTTP 処理をまとめたパッケージ。
package task

import (
	"errors"
	"strings"
	"time"
)

// Task は 1 件の TODO を表す。
// 構造体タグ (`json:"..."`) は JSON にした時のフィールド名を決める。
// タグを書かないとフィールド名がそのまま（Title -> "Title"）使われてしまう。
type Task struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrNotFound は指定 ID のタスクが存在しないことを表す番兵エラー。
// 呼び出し側は errors.Is(err, task.ErrNotFound) で判定する。
var ErrNotFound = errors.New("task not found")

// ErrInvalidTitle はタイトルが不正なときのエラー。
var ErrInvalidTitle = errors.New("title must be 1-200 characters")

const maxTitleLen = 200

// normalizeTitle は前後の空白を取り除き、妥当かどうかを検証する。
func normalizeTitle(title string) (string, error) {
	t := strings.TrimSpace(title)
	if t == "" || len([]rune(t)) > maxTitleLen {
		return "", ErrInvalidTitle
	}
	return t, nil
}
