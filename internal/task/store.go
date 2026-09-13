package task

import (
	"sort"
	"sync"
	"time"
)

// Store はタスクをメモリ上に保持する。プロセスを再起動すると中身は消える。
// HTTP サーバーは複数の goroutine から同時にハンドラを呼ぶので、
// map をそのまま触ると data race になる。sync.RWMutex で保護する。
type Store struct {
	mu     sync.RWMutex
	tasks  map[int]Task
	nextID int

	// now は時刻取得を差し替えられるようにしておくためのフィールド（テスト用）。
	now func() time.Time
}

// NewStore は空の Store を作る。
func NewStore() *Store {
	return &Store{
		tasks:  make(map[int]Task),
		nextID: 1,
		now:    time.Now,
	}
}

// List は全タスクを ID 昇順で返す。
func (s *Store) List() []Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		out = append(out, t)
	}
	// map の反復順序は Go では毎回ランダムなので、明示的に並べ替える。
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get は 1 件取得する。無ければ ErrNotFound を返す。
func (s *Store) Get(id int) (Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tasks[id]
	if !ok {
		return Task{}, ErrNotFound
	}
	return t, nil
}

// Create は新しいタスクを追加して、追加後のタスクを返す。
func (s *Store) Create(title string) (Task, error) {
	title, err := normalizeTitle(title)
	if err != nil {
		return Task{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t := Task{
		ID:        s.nextID,
		Title:     title,
		Done:      false,
		CreatedAt: s.now().UTC(),
	}
	s.tasks[t.ID] = t
	s.nextID++
	return t, nil
}

// Update は部分更新を行う。nil のフィールドは変更しない（PATCH 相当）。
func (s *Store) Update(id int, title *string, done *bool) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return Task{}, ErrNotFound
	}

	if title != nil {
		normalized, err := normalizeTitle(*title)
		if err != nil {
			return Task{}, err
		}
		t.Title = normalized
	}
	if done != nil {
		t.Done = *done
	}

	// t は map から取り出したコピーなので、書き戻さないと反映されない。
	s.tasks[id] = t
	return t, nil
}

// Delete は 1 件削除する。無ければ ErrNotFound を返す。
func (s *Store) Delete(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[id]; !ok {
		return ErrNotFound
	}
	delete(s.tasks, id)
	return nil
}
