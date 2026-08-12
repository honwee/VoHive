package sipgw

import (
	"sync"
	"testing"
	"time"
)

// memBindingStore 是 BindingStore 的内存实现，用来在不碰 SQLite 的前提下验证恢复
// 逻辑——这也顺便证明了接口注入是值得的：如果 registrar 直接 import internal/db，
// 这些测试就得起一个数据库。
type memBindingStore struct {
	mu   sync.Mutex
	rows map[string]PersistedBinding
	// loadErr 用来模拟存储不可用。
	loadErr error
}

func newMemStore() *memBindingStore {
	return &memBindingStore{rows: make(map[string]PersistedBinding)}
}

func (m *memBindingStore) Save(b PersistedBinding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[b.Username] = b
	return nil
}

func (m *memBindingStore) Delete(username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, username)
	return nil
}

func (m *memBindingStore) LoadLive(now time.Time) ([]PersistedBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	var out []PersistedBinding
	for _, b := range m.rows {
		if b.ExpiresAt.After(now) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *memBindingStore) PurgeExpired(now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, b := range m.rows {
		if !b.ExpiresAt.After(now) {
			delete(m.rows, k)
		}
	}
	return nil
}

func newTestRegistrar(t *testing.T) *Registrar {
	t.Helper()
	r, err := NewRegistrar(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRegistrar: %v", err)
	}
	return r
}

// 重启后必须能立刻按设备找到软电话，否则这段窗口里的来话全部落空。
func TestRestoreBindingsMakesDeviceReachable(t *testing.T) {
	store := newMemStore()
	_ = store.Save(PersistedBinding{
		Username:    "dji4g",
		DeviceID:    "dji4g_1",
		ContactURI:  "sip:dji4g@192.168.64.1:53427",
		ContactAddr: "192.168.64.1:53427",
		Transport:   "udp",
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	})

	r := newTestRegistrar(t)
	r.SetBindingStore(store)
	r.restoreBindings()

	uri, addr, username, err := r.GetClientContact("dji4g_1")
	if err != nil {
		t.Fatalf("GetClientContact after restore: %v", err)
	}
	if username != "dji4g" {
		t.Errorf("username = %q", username)
	}
	if addr != "192.168.64.1:53427" {
		t.Errorf("contact addr = %q, 来话会发到错误的地方", addr)
	}
	if uri == "" {
		t.Errorf("contact URI 丢失")
	}
}

// 过期的绑定不能复活：投递到一个早就走了的地址，比明确知道没人在更糟——它会占满
// 整个振铃超时。
func TestRestoreBindingsSkipsExpired(t *testing.T) {
	store := newMemStore()
	_ = store.Save(PersistedBinding{
		Username:    "stale",
		DeviceID:    "dev-stale",
		ContactAddr: "192.168.64.1:1111",
		ExpiresAt:   time.Now().Add(-time.Minute),
	})

	r := newTestRegistrar(t)
	r.SetBindingStore(store)
	r.restoreBindings()

	if _, _, _, err := r.GetClientContact("dev-stale"); err == nil {
		t.Fatal("过期绑定被恢复了")
	}
}

// 存储不可用不能让 registrar 起不来：软电话下次刷新时会自己回来，那是可恢复的；
// 起不来不是。
func TestRestoreBindingsToleratesStoreFailure(t *testing.T) {
	store := newMemStore()
	store.loadErr = errStoreDown{}

	r := newTestRegistrar(t)
	r.SetBindingStore(store)
	r.restoreBindings() // 不能 panic

	if _, _, _, err := r.GetClientContact("anything"); err == nil {
		t.Fatal("不该凭空得到一个绑定")
	}
}

type errStoreDown struct{}

func (errStoreDown) Error() string { return "store down" }

// 没有 store 时行为必须与改动前完全一致：纯内存。
func TestNilStoreKeepsInMemoryBehaviour(t *testing.T) {
	r := newTestRegistrar(t)
	r.restoreBindings() // 不能 panic
	r.persistBinding(&RegisteredUser{Username: "x"})
	r.purgePersistedBindings(time.Now())
}

// 注册一次就该落盘，否则重启后仍是黑洞。
func TestRegisterUserPersists(t *testing.T) {
	store := newMemStore()
	r := newTestRegistrar(t)
	r.SetBindingStore(store)

	r.registerUser("dji4g", "dji4g_1", "DJI 4G", "sip:dji4g@192.168.64.1:5060",
		nil, "udp", "baresip", 600, "", "", "", "", "")

	rows, err := store.LoadLive(time.Now())
	if err != nil {
		t.Fatalf("LoadLive: %v", err)
	}
	if len(rows) != 1 || rows[0].Username != "dji4g" {
		t.Fatalf("注册未落盘: %+v", rows)
	}
	if rows[0].DeviceID != "dji4g_1" {
		t.Errorf("device id 未保存，恢复后按设备查不到")
	}
}

func TestPurgeExpiredDropsOldRows(t *testing.T) {
	store := newMemStore()
	_ = store.Save(PersistedBinding{Username: "old", ExpiresAt: time.Now().Add(-time.Hour)})
	_ = store.Save(PersistedBinding{Username: "live", ExpiresAt: time.Now().Add(time.Hour)})

	r := newTestRegistrar(t)
	r.SetBindingStore(store)
	r.purgePersistedBindings(time.Now())

	rows, _ := store.LoadLive(time.Now().Add(-2 * time.Hour))
	if len(rows) != 1 || rows[0].Username != "live" {
		t.Fatalf("过期条目未被清理: %+v", rows)
	}
}
