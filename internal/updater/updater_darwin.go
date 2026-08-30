//go:build darwin

package updater

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework AppKit
#include <stdbool.h>
bool AetherStartSparkle(void);
bool AetherSparkleAvailable(void);
*/
import "C"

func Start() bool     { return bool(C.AetherStartSparkle()) }
func Available() bool { return bool(C.AetherSparkleAvailable()) }
