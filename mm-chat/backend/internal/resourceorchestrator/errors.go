package resourceorchestrator

import "errors"

var ErrInvalidQuery = errors.New("resource query is invalid")
var ErrRevisionChanged = errors.New("resource revision changed")
var ErrConfigurationRequired = errors.New("resource configuration is required")
var ErrDisabled = errors.New("resource orchestration is disabled")
var ErrUnavailable = errors.New("resource source is unavailable")
var ErrAuditUnavailable = errors.New("resource mutation audit is unavailable")
