package models

import (
	"errors"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
)

var (
	ErrNotFound      = db.ErrNotFound
	ErrAlreadyExists = db.ErrAlreadyExists
	ErrConflict      = db.ErrConflict
	ErrUnauthorized  = errors.New("invalid credentials")
	ErrForbidden     = errors.New("forbidden")
	ErrBadRequest    = errors.New("bad request")
	ErrBadGateway    = errors.New("bad gateway")
)
