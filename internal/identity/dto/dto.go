package dto

// UpdateProfileRequest is the body for PUT /users/me.
type UpdateProfileRequest struct {
	DisplayName string  `json:"display_name" validate:"omitempty,min=1,max=100"`
	Timezone    string  `json:"timezone"     validate:"omitempty,min=1,max=64"`
	Locale      string  `json:"locale"       validate:"omitempty,min=2,max=10"`
	AvatarURL   *string `json:"avatar_url"   validate:"omitempty,url"`
}

// DeleteAccountRequest is the body for DELETE /users/me.
// Email+password accounts must supply their current password.
// OAuth-only accounts (no password) must supply the exact confirmation string.
type DeleteAccountRequest struct {
	Password     string `json:"password"`
	Confirmation string `json:"confirmation"`
}

// UpdatePreferencesRequest is the body for PUT /users/me/preferences.
// String fields use empty string as "not provided" so oneof validation fires correctly.
// Bool fields use pointers to distinguish "not provided" from false.
type UpdatePreferencesRequest struct {
	DistanceUnit             string   `json:"distance_unit"              validate:"omitempty,oneof=km miles"`
	PushNotificationsEnabled *bool    `json:"push_notifications_enabled"`
	WeeklyReportEnabled      *bool    `json:"weekly_report_enabled"`
	DailyGoalKgCO2e          *float64 `json:"daily_goal_kg_co2e"         validate:"omitempty,gt=0"`
	DataSharingEnabled       *bool    `json:"data_sharing_enabled"`
	HomeAddress              *string  `json:"home_address"               validate:"omitempty,max=500"`
	HomeLat                  *float64 `json:"home_lat"`
	HomeLng                  *float64 `json:"home_lng"`
	WorkAddress              *string  `json:"work_address"               validate:"omitempty,max=500"`
	WorkLat                  *float64 `json:"work_lat"`
	WorkLng                  *float64 `json:"work_lng"`
	DefaultTransport         *string  `json:"default_transport"          validate:"omitempty,max=50"`
}
