//go:build !darwin

package updater

func Start() bool     { return false }
func Available() bool { return false }
