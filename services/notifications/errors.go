package notifications

import "errors"

var (
	ErrTemplateNotFound   = errors.New("notifications: template not found for event/channel")
	ErrTemplateRender     = errors.New("notifications: template render failed")
	ErrInvalidChannel     = errors.New("notifications: invalid channel; must be email, sms, in_app, or push")
	ErrNoSenderForChannel = errors.New("notifications: no sender configured for channel")
	ErrNotificationNotFound = errors.New("notifications: notification not found")
)
