package iam

import "errors"

var (
	ErrInvalidToken    = errors.New("iam: invalid or expired token")
	ErrNotImplemented  = errors.New("iam: not implemented")
	ErrTenantSuspended = errors.New("iam: tenant suspended")
)
