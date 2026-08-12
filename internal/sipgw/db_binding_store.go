package sipgw

import (
	"time"

	"github.com/1239t/vohive/internal/db"
)

// DBBindingStore 是 BindingStore 的 SQLite 实现。
//
// 它住在这里而不是 internal/db，是为了让 registrar 只依赖自己定义的接口：包本身
// 不需要知道存储形态，测试可以塞一个内存实现。
type DBBindingStore struct{}

func (DBBindingStore) Save(b PersistedBinding) error {
	return db.UpsertSIPBinding(db.SIPBinding{
		Username:     b.Username,
		DeviceID:     b.DeviceID,
		DisplayName:  b.DisplayName,
		ContactURI:   b.ContactURI,
		ContactAddr:  b.ContactAddr,
		Transport:    b.Transport,
		UserAgent:    b.UserAgent,
		ExpiresAt:    b.ExpiresAt,
		PushToken:    b.PushToken,
		PushProvider: b.PushProvider,
		PushParam:    b.PushParam,
		PushCallStr:  b.PushCallStr,
		PushMsgStr:   b.PushMsgStr,
	})
}

func (DBBindingStore) Delete(username string) error { return db.DeleteSIPBinding(username) }

func (DBBindingStore) LoadLive(now time.Time) ([]PersistedBinding, error) {
	rows, err := db.LoadLiveSIPBindings(now)
	if err != nil {
		return nil, err
	}
	out := make([]PersistedBinding, 0, len(rows))
	for _, r := range rows {
		out = append(out, PersistedBinding{
			Username:     r.Username,
			DeviceID:     r.DeviceID,
			DisplayName:  r.DisplayName,
			ContactURI:   r.ContactURI,
			ContactAddr:  r.ContactAddr,
			Transport:    r.Transport,
			UserAgent:    r.UserAgent,
			ExpiresAt:    r.ExpiresAt,
			PushToken:    r.PushToken,
			PushProvider: r.PushProvider,
			PushParam:    r.PushParam,
			PushCallStr:  r.PushCallStr,
			PushMsgStr:   r.PushMsgStr,
		})
	}
	return out, nil
}

func (DBBindingStore) PurgeExpired(now time.Time) error { return db.PurgeExpiredSIPBindings(now) }
