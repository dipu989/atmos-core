package dto

// UpdateProfileRequest is the body for PUT /users/me.
type UpdateProfileRequest struct {
	DisplayName string  `json:"display_name" validate:"omitempty,min=1,max=100"`
	Timezone    string  `json:"timezone"     validate:"omitempty,min=1,max=64"`
	Locale      string  `json:"locale"       validate:"omitempty,min=2,max=10"`
	AvatarURL   *string `json:"avatar_url"   validate:"omitempty,url"`
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
}
