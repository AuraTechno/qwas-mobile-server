package models

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"displayName"`
	Bio          string    `json:"bio"`
	AvatarURL    string    `json:"avatarUrl"`
	AvatarColor  string    `json:"avatarColor"`
	IsOnline     bool      `json:"isOnline"`
	LastSeen     time.Time `json:"lastSeen"`
	CreatedAt    time.Time `json:"createdAt"`
}

type UserWithPassword struct {
	User
	PasswordHash string
}

type Chat struct {
	ID              int64     `json:"id"`
	Type            string    `json:"type"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	AvatarURL       string    `json:"avatarUrl"`
	AvatarColor     string    `json:"avatarColor"`
	OwnerID         int64     `json:"ownerId"`
	PinnedMessageID *int64    `json:"pinnedMessageId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type ChatMember struct {
	ChatID    int64  `json:"chatId"`
	UserID    int64  `json:"userId"`
	Role      string `json:"role"`
	IsMuted   bool   `json:"isMuted"`
	NotifsEnabled bool `json:"notificationsEnabled"`
	LastReadMessageID *int64 `json:"lastReadMessageId,omitempty"`
	JoinedAt  time.Time `json:"joinedAt"`
}

type Message struct {
	ID          int64     `json:"id"`
	ChatID      int64     `json:"chatId"`
	SenderID    int64     `json:"senderId"`
	SenderName  string    `json:"senderName,omitempty"`
	SenderColor string    `json:"senderColor,omitempty"`
	Type        string    `json:"type"`
	Content     string    `json:"content"`
	MediaURL    string    `json:"mediaUrl,omitempty"`
	MediaMeta   string    `json:"mediaMeta,omitempty"` // JSON string
	ReplyToID   *int64    `json:"replyToId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	EditedAt    *time.Time `json:"editedAt,omitempty"`
	IsDeleted   bool      `json:"isDeleted"`
}

type Session struct {
	ID         string    `json:"id"`
	UserID     int64     `json:"userId"`
	TokenHash  string    `json:"-"`
	DeviceInfo string    `json:"deviceInfo"`
	IP         string    `json:"ip"`
	LastActive time.Time `json:"lastActive"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Reaction struct {
	MessageID int64     `json:"messageId"`
	UserID    int64     `json:"userId"`
	Emoji     string    `json:"emoji"`
	CreatedAt time.Time `json:"createdAt"`
}
