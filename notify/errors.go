package notify

import "errors"

// ErrDeliver is returned when a channel rejects or cannot receive a notification.
var ErrDeliver = errors.New("notification delivery failed")
