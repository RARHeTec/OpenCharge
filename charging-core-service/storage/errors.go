package storage

import "errors"

var (
	ErrWalletNotFound = errors.New("storage: wallet not found")
	ErrNotFound       = errors.New("storage: not found")
	ErrInsufficient   = errors.New("storage: insufficient balance")
)
