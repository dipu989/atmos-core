package dto

// RegisterDeviceRequest is the body for POST /devices/register.
type RegisterDeviceRequest struct {
	DeviceToken     string  `json:"device_token"     validate:"required,min=1,max=512"`
	Platform        string  `json:"platform"         validate:"required,oneof=ios ipados android web unknown"`
	PushProvider    string  `json:"push_provider"    validate:"omitempty,oneof=apns fcm none"`
	APNsEnvironment *string `json:"apns_environment" validate:"omitempty,oneof=sandbox production"`
	DeviceName      *string `json:"device_name"      validate:"omitempty,max=100"`
	OSVersion       *string `json:"os_version"       validate:"omitempty,max=50"`
	AppVersion      *string `json:"app_version"      validate:"omitempty,max=50"`
	PushToken       *string `json:"push_token"       validate:"omitempty,min=1,max=512"`
}

// UpdatePushTokenRequest is the body for PATCH /devices/:id.
type UpdatePushTokenRequest struct {
	PushToken  string  `json:"push_token"  validate:"required,min=1,max=512"`
	AppVersion *string `json:"app_version" validate:"omitempty,max=50"`
}
