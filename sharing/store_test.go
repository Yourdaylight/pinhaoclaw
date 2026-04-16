package sharing

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentSaveUsers(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	var wg sync.WaitGroup
	count := 100
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			u := &User{
				ID:          "user_" + string(rune('A'+idx%26)) + string(rune('0'+idx%10)),
				Name:        "测试用户",
				MaxLobsters: 3,
			}
			if err := store.SaveUser(u); err != nil {
				t.Errorf("SaveUser(%s) failed: %v", u.ID, err)
			}
		}(i)
	}
	wg.Wait()

	users, err := store.ReadUsers()
	if err != nil {
		t.Fatalf("ReadUsers failed: %v", err)
	}
	if len(users) != count {
		t.Errorf("expected %d users, got %d", count, len(users))
	}
}

func TestConcurrentSaveLobsters(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	var wg sync.WaitGroup
	count := 100
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			l := &Lobster{
				ID:     "lobster_" + string(rune('A'+idx%26)) + string(rune('0'+idx%10)),
				UserID: "user_test",
				Name:   "测试龙虾",
			}
			if err := store.SaveLobster(l); err != nil {
				t.Errorf("SaveLobster(%s) failed: %v", l.ID, err)
			}
		}(i)
	}
	wg.Wait()

	lobsters, err := store.ReadLobsters()
	if err != nil {
		t.Fatalf("ReadLobsters failed: %v", err)
	}
	if len(lobsters) != count {
		t.Errorf("expected %d lobsters, got %d", count, len(lobsters))
	}
}

func TestReadDuringWrite(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// 写入初始数据
	for i := 0; i < 10; i++ {
		store.SaveUser(&User{ID: "user_init_" + string(rune('A'+i)), Name: "init"})
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// 写者: 持续写入
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			store.SaveUser(&User{ID: "user_writer", Name: "writer"})
		}
	}()

	// 读者: 持续读取，不应得到空数据或错误
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			users, err := store.ReadUsers()
			if err != nil {
				t.Errorf("ReadUsers failed during write: %v", err)
				return
			}
			if len(users) == 0 {
				t.Error("ReadUsers returned empty during write, data lost")
				return
			}
		}
	}()

	wg.Wait()
}

func TestWithLockAtomicUpdate(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// 创建初始用户
	store.SaveUser(&User{ID: "user_atomic", Name: "原子测试", MaxLobsters: 1})

	var wg sync.WaitGroup
	count := 50
	wg.Add(count)

	// 并发原子递增 MaxLobsters
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			err := store.WithLock("users.json", func() error {
				users := make(map[string]*User)
				_ = store.readFile("users.json", &users)
				u := users["user_atomic"]
				if u == nil {
					return nil
				}
				u.MaxLobsters++
				return store.writeFile("users.json", users)
			})
			if err != nil {
				t.Errorf("WithLock failed: %v", err)
			}
		}()
	}
	wg.Wait()

	users, _ := store.ReadUsers()
	u := users["user_atomic"]
	if u == nil {
		t.Fatal("user_atomic not found")
	}
	// 初始 1 + 50 次递增 = 51
	if u.MaxLobsters != 1+count {
		t.Errorf("expected MaxLobsters=%d, got %d (race condition detected)", 1+count, u.MaxLobsters)
	}
}

func TestConcurrentSaveNodes(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	var wg sync.WaitGroup
	count := 50
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			n := &Node{
				ID:     "node_" + string(rune('A'+idx%26)) + string(rune('0'+idx%10)),
				Name:   "测试节点",
				Host:   "192.168.1.1",
				Region: "华南",
			}
			if err := store.SaveNode(n); err != nil {
				t.Errorf("SaveNode(%s) failed: %v", n.ID, err)
			}
		}(i)
	}
	wg.Wait()

	nodes, _ := store.ReadNodes()
	if len(nodes) != count {
		t.Errorf("expected %d nodes, got %d", count, len(nodes))
	}
}

func TestFileLockPreventsCorruption(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// 写入有效 JSON
	store.SaveUser(&User{ID: "user_1", Name: "测试"})

	// 并发大量写入，验证文件始终可解析
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			u := &User{ID: "user_1", Name: "更新" + string(rune('A'+idx%26)), MaxLobsters: idx}
			store.SaveUser(u)
		}(i)
	}
	wg.Wait()

	// 验证文件可正确解析
	users, err := store.ReadUsers()
	if err != nil {
		t.Fatalf("ReadUsers after concurrent writes failed: %v", err)
	}
	if len(users) == 0 {
		t.Fatal("no users after concurrent writes")
	}

	// 验证文件内容是合法 JSON
	data, err := os.ReadFile(filepath.Join(dir, "users.json"))
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("users.json is empty")
	}
}
