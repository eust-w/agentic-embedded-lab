//go:build darwin && cgo

package launchagent

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework ServiceManagement
#import <Foundation/Foundation.h>
#import <ServiceManagement/ServiceManagement.h>
#include <stdlib.h>

static const char *aether_copy_error(NSError *error) {
  if (!error) return NULL;
  return strdup(error.localizedDescription.UTF8String);
}

static int aether_service_register(const char *plist, const char **error_message) {
  @autoreleasepool {
    NSString *name = [NSString stringWithUTF8String:plist];
    SMAppService *service = [SMAppService agentServiceWithPlistName:name];
    NSError *error = nil;
    BOOL success = [service registerAndReturnError:&error];
    if (!success && error_message) *error_message = aether_copy_error(error);
    return success ? 0 : 1;
  }
}

static int aether_service_unregister(const char *plist, const char **error_message) {
  @autoreleasepool {
    NSString *name = [NSString stringWithUTF8String:plist];
    SMAppService *service = [SMAppService agentServiceWithPlistName:name];
    NSError *error = nil;
    BOOL success = [service unregisterAndReturnError:&error];
    if (!success && error_message) *error_message = aether_copy_error(error);
    return success ? 0 : 1;
  }
}

static long aether_service_status(const char *plist) {
  @autoreleasepool {
    NSString *name = [NSString stringWithUTF8String:plist];
    return (long)[SMAppService agentServiceWithPlistName:name].status;
  }
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

const DefaultPlist = "dev.aether.desktop.daemon.plist"

type MacService struct{ PlistName string }

func New() Service { return &MacService{PlistName: DefaultPlist} }

func (s *MacService) Register() error {
	name := C.CString(s.PlistName)
	defer C.free(unsafe.Pointer(name))
	var message *C.char
	if C.aether_service_register(name, &message) == 0 {
		return nil
	}
	return serviceError(message)
}

func (s *MacService) Unregister() error {
	name := C.CString(s.PlistName)
	defer C.free(unsafe.Pointer(name))
	var message *C.char
	if C.aether_service_unregister(name, &message) == 0 {
		return nil
	}
	return serviceError(message)
}

func (s *MacService) Status() Status {
	name := C.CString(s.PlistName)
	defer C.free(unsafe.Pointer(name))
	switch int64(C.aether_service_status(name)) {
	case 0:
		return StatusNotRegistered
	case 1:
		return StatusEnabled
	case 2:
		return StatusRequiresApproval
	default:
		return StatusNotFound
	}
}

func serviceError(message *C.char) error {
	if message == nil {
		return errors.New("SMAppService operation failed")
	}
	defer C.free(unsafe.Pointer(message))
	return errors.New(C.GoString(message))
}
