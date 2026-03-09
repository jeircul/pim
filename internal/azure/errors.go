package azure

import "errors"

var (
	ErrNoItems       = errors.New("no items")
	ErrUserCancelled = errors.New("user cancelled")
	ErrNoCredential  = errors.New("no supported Azure login found; sign in with 'az login' or 'Connect-AzAccount'")
)
