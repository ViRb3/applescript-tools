//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Carbon
#include <Carbon/Carbon.h>
#include <stdlib.h>

static int extract_system_terminology(void **output, long *output_size) {
	ComponentInstance component = OpenDefaultComponent(kOSAComponentType, 0x61736372);
	if (component == NULL) return -1;
	AEDesc terminology = {typeNull, NULL};
	OSAError error = OSAGetSysTerminology(component, 0, 0, &terminology);
	if (error != noErr) {
		CloseComponent(component);
		return error;
	}
	Size size = AEGetDescDataSize(&terminology);
	void *data = malloc((size_t)size);
	if (data == NULL) {
		AEDisposeDesc(&terminology);
		CloseComponent(component);
		return -1;
	}
	error = AEGetDescData(&terminology, data, size);
	AEDisposeDesc(&terminology);
	CloseComponent(component);
	if (error != noErr) {
		free(data);
		return error;
	}
	*output = data;
	*output_size = size;
	return 0;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func extractSystemTerminology() ([]byte, error) {
	var data unsafe.Pointer
	var size C.long
	if status := C.extract_system_terminology(&data, &size); status != 0 {
		return nil, fmt.Errorf("OSAGetSysTerminology failed: %d", int(status))
	}
	defer C.free(data)
	return C.GoBytes(data, C.int(size)), nil
}
