//go:build !windows

package clicker

import "errors"

var errUnsupported = errors.New("连点器只支持 Windows")

type stubDriver struct{}

func newDriver() driver { return stubDriver{} }

func (stubDriver) press(string, uint16, bool) error { return errUnsupported }
func (stubDriver) gameFocused() bool                { return false }
func (stubDriver) registerHotkey(uint16, func()) (func(), error) {
	return nil, errUnsupported
}
