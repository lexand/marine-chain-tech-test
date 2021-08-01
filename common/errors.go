package common

import "errors"

var (
	ErrBadPath             = errors.New("bad path for storing files")
	ErrFileNameEmpty       = errors.New("file name can not be empty")
	ErrCantRegister        = errors.New("cant register self in public instance")
	ErrBadPrivateId        = errors.New("bad ID, expect len(id) == 4")
	ErrExpectHttpNoContent = errors.New("on no error expect http 204 no content ")
	ErrExpectHttpOK        = errors.New("on no error expect http 200 OK ")
)
