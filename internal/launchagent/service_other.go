//go:build !darwin || !cgo

package launchagent

import "errors"

type unavailable struct{}

func New() Service                    { return unavailable{} }
func (unavailable) Register() error   { return errors.New("SMAppService is unavailable") }
func (unavailable) Unregister() error { return errors.New("SMAppService is unavailable") }
func (unavailable) Status() Status    { return StatusNotFound }
