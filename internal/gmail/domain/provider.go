package domain

import "time"

// EmissionCategory is a node in the emission taxonomy tree.
// Leaves map to a specific activity_type in the activities table.
type EmissionCategory struct {
	Code         string    `gorm:"primaryKey"           json:"code"`
	ParentCode   *string   `json:"parent_code,omitempty"`
	DisplayName  string    `gorm:"not null"             json:"display_name"`
	ActivityType string    `gorm:"not null"             json:"activity_type"`
	Icon         *string   `json:"icon,omitempty"`
	CreatedAt    time.Time `json:"created_at"`

	// populated by JOIN when needed
	Children []*EmissionCategory `gorm:"-" json:"children,omitempty"`
}

func (EmissionCategory) TableName() string { return "emission_categories" }

// Provider is the company/brand whose emails we parse.
type Provider struct {
	Code        string    `gorm:"primaryKey" json:"code"`
	DisplayName string    `gorm:"not null"   json:"display_name"`
	LogoURL     *string   `json:"logo_url,omitempty"`
	Country     string    `gorm:"not null"   json:"country"`
	IsActive    bool      `gorm:"not null"   json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

func (Provider) TableName() string { return "providers" }

// ProviderEmailType is one parseable email format from a Provider.
// The Registry maps Code → Parser implementation.
type ProviderEmailType struct {
	Code           string    `gorm:"primaryKey" json:"code"`
	ProviderCode   string    `gorm:"not null"   json:"provider_code"`
	DisplayName    string    `gorm:"not null"   json:"display_name"`
	CategoryCode   string    `gorm:"not null"   json:"category_code"`
	SenderEmail    string    `gorm:"not null"   json:"sender_email"`
	SubjectPattern *string   `json:"subject_pattern,omitempty"`
	IsActive       bool      `gorm:"not null"   json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`

	// Populated by JOIN when needed
	Provider *Provider         `gorm:"-" json:"provider,omitempty"`
	Category *EmissionCategory `gorm:"-" json:"category,omitempty"`
}

func (ProviderEmailType) TableName() string { return "provider_email_types" }
