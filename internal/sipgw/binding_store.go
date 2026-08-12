package sipgw

import (
	"net"
	"time"

	"github.com/1239t/vohive/pkg/logger"
)

// 注册绑定的持久化。
//
// registrar 的注册表原本纯内存：vohive 一重启就清空，而软电话的 Expires 通常是
// 600 秒，它要到自己下次刷新才回来。这中间最长十分钟，每一通来话都以"软电话未注册"
// 失败——**而软电话界面显示"已注册"**，用户完全无从察觉。实测中就这样丢过一通电话。
//
// 存储以接口注入而不是直接 import internal/db：这个包是一个 SIP registrar，不该知道
// 数据落在 SQLite 还是别处，而且没有存储时（测试、或调用方不关心持久化）必须仍然
// 能用。nil store 就是纯内存行为，与改动前完全一致。
type BindingStore interface {
	Save(b PersistedBinding) error
	Delete(username string) error
	LoadLive(now time.Time) ([]PersistedBinding, error)
	PurgeExpired(now time.Time) error
}

// PersistedBinding 是一条绑定里值得跨重启保留的部分。
//
// 刻意不含任何凭据：恢复出来的绑定只是"上次这个用户从哪里可达"，不是免检通行证。
// 软电话下次 REGISTER 时照样要过 digest 认证。
type PersistedBinding struct {
	Username    string
	DeviceID    string
	DisplayName string
	ContactURI  string
	ContactAddr string
	Transport   string
	UserAgent   string
	ExpiresAt   time.Time

	PushToken    string
	PushProvider string
	PushParam    string
	PushCallStr  string
	PushMsgStr   string
}

// SetBindingStore 注入持久化后端。要在 Start 之前调用才能参与启动恢复。
func (r *Registrar) SetBindingStore(s BindingStore) {
	r.mu.Lock()
	r.bindingStore = s
	r.mu.Unlock()
}

// restoreBindings 用未过期的绑定重建内存表。
//
// 恢复的是**可达性**，不是在线状态：这些地址可能已经不通了。这是可以接受的——投递到
// 一个陈旧的 Contact 最坏是超时，然后按无人接听处理；而完全没有绑定则是连试都不试，
// 直接把来话推给语音信箱。前者可恢复，后者是静默的黑洞。
func (r *Registrar) restoreBindings() {
	r.mu.RLock()
	store := r.bindingStore
	r.mu.RUnlock()
	if store == nil {
		return
	}

	now := time.Now()
	rows, err := store.LoadLive(now)
	if err != nil {
		logger.Warn("恢复 SIP 注册绑定失败，将等待软电话自行重新注册", "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	restored := 0
	r.mu.Lock()
	for _, b := range rows {
		if b.Username == "" {
			continue
		}
		user := &RegisteredUser{
			Username:     b.Username,
			DeviceID:     b.DeviceID,
			DisplayName:  b.DisplayName,
			ContactURI:   b.ContactURI,
			Transport:    b.Transport,
			UserAgent:    b.UserAgent,
			Expires:      b.ExpiresAt,
			Source:       b.ContactAddr,
			PushToken:    b.PushToken,
			PushProvider: b.PushProvider,
			PushParam:    b.PushParam,
			PushCallStr:  b.PushCallStr,
			PushMsgStr:   b.PushMsgStr,
		}
		if addr, addrErr := net.ResolveUDPAddr("udp", b.ContactAddr); addrErr == nil {
			user.ContactAddr = addr
		}
		r.users[b.Username] = user
		if b.DeviceID != "" {
			r.byDevice[b.DeviceID] = user
			if _, ok := r.onlineSignals[b.DeviceID]; !ok {
				r.onlineSignals[b.DeviceID] = make(chan struct{})
			}
		}
		restored++
	}
	r.mu.Unlock()

	logger.Info("已从持久化恢复 SIP 注册绑定", "count", restored)
}

// persistBinding 落盘一条绑定。失败只告警：注册本身已经成功，持久化不成功只意味着
// 下次重启后要多等软电话一个刷新周期，不该反过来让这次注册失败。
func (r *Registrar) persistBinding(user *RegisteredUser) {
	if user == nil {
		return
	}
	r.mu.RLock()
	store := r.bindingStore
	r.mu.RUnlock()
	if store == nil {
		return
	}

	addr := ""
	if user.ContactAddr != nil {
		addr = user.ContactAddr.String()
	}
	if err := store.Save(PersistedBinding{
		Username:     user.Username,
		DeviceID:     user.DeviceID,
		DisplayName:  user.DisplayName,
		ContactURI:   user.ContactURI,
		ContactAddr:  addr,
		Transport:    user.Transport,
		UserAgent:    user.UserAgent,
		ExpiresAt:    user.Expires,
		PushToken:    user.PushToken,
		PushProvider: user.PushProvider,
		PushParam:    user.PushParam,
		PushCallStr:  user.PushCallStr,
		PushMsgStr:   user.PushMsgStr,
	}); err != nil {
		logger.Warn("持久化 SIP 注册绑定失败", "username", user.Username, "err", err)
	}
}

// purgePersistedBindings 让盘上的过期条目跟着内存清理一起走，避免无限增长。
func (r *Registrar) purgePersistedBindings(now time.Time) {
	r.mu.RLock()
	store := r.bindingStore
	r.mu.RUnlock()
	if store == nil {
		return
	}
	if err := store.PurgeExpired(now); err != nil {
		logger.Warn("清理过期 SIP 注册绑定失败", "err", err)
	}
}
