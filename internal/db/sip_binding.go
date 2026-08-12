package db

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SIPBinding 是软电话的注册绑定。
//
// 这张表存在的理由是一个具体的黑洞：registrar 的注册表原本纯内存，vohive 一重启就
// 清空，而软电话的 Expires 通常是 600 秒——它要到自己下次刷新才会回来。这中间最长
// 十分钟，每一通来话都以"软电话未注册"失败，**而软电话界面显示"已注册"**，用户完全
// 无从察觉。实测中就这样丢过一通电话。
//
// 只存重建可达性所必需的字段。凭据不在其列：注册仍然要过 digest 认证，恢复出来的
// 绑定只是"上次这个用户从哪里可达"，不是一张免检通行证。
type SIPBinding struct {
	Username    string    `gorm:"column:username;primaryKey" json:"username"`
	DeviceID    string    `gorm:"column:device_id;index" json:"device_id"`
	DisplayName string    `gorm:"column:display_name" json:"display_name"`
	ContactURI  string    `gorm:"column:contact_uri" json:"contact_uri"`
	ContactAddr string    `gorm:"column:contact_addr" json:"contact_addr"`
	Transport   string    `gorm:"column:transport" json:"transport"`
	UserAgent   string    `gorm:"column:user_agent" json:"user_agent"`
	ExpiresAt   time.Time `gorm:"column:expires_at;index" json:"expires_at"`

	// 推送参数：iOS 上软电话会被系统挂起，没有它们就只能投递到一个已经不听的
	// socket，然后等超时。
	PushToken    string `gorm:"column:push_token" json:"push_token"`
	PushProvider string `gorm:"column:push_provider" json:"push_provider"`
	PushParam    string `gorm:"column:push_param" json:"push_param"`
	PushCallStr  string `gorm:"column:push_call_str" json:"push_call_str"`
	PushMsgStr   string `gorm:"column:push_msg_str" json:"push_msg_str"`

	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (SIPBinding) TableName() string { return "sip_bindings" }

// UpsertSIPBinding 记录一次注册。按 username 覆盖：一个账号同时只有一个绑定，
// 这与 registrar 内存表的语义一致。
func UpsertSIPBinding(b SIPBinding) error {
	if DB == nil || b.Username == "" {
		return nil
	}
	b.UpdatedAt = time.Now()
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "username"}},
		UpdateAll: true,
	}).Create(&b).Error
}

// DeleteSIPBinding 注销时移除。
func DeleteSIPBinding(username string) error {
	if DB == nil || username == "" {
		return nil
	}
	return DB.Where("username = ?", username).Delete(&SIPBinding{}).Error
}

// LoadLiveSIPBindings 返回尚未过期的绑定，供启动时恢复。
//
// 过期的不返回也不删除：清理交给 registrar 既有的定期 cleanup，让"什么时候算过期"
// 只有一处定义。
func LoadLiveSIPBindings(now time.Time) ([]SIPBinding, error) {
	if DB == nil {
		return nil, nil
	}
	var out []SIPBinding
	err := DB.Where("expires_at > ?", now).Find(&out).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return out, nil
}

// PurgeExpiredSIPBindings 删除已过期的绑定。
func PurgeExpiredSIPBindings(now time.Time) error {
	if DB == nil {
		return nil
	}
	return DB.Where("expires_at <= ?", now).Delete(&SIPBinding{}).Error
}
