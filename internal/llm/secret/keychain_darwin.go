//go:build darwin && cgo

package secret

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <stdlib.h>
static CFMutableDictionaryRef steward_query(const char *ref) {
 CFMutableDictionaryRef q=CFDictionaryCreateMutable(NULL,0,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 CFStringRef account=CFStringCreateWithCString(NULL,ref,kCFStringEncodingUTF8);
 CFDictionarySetValue(q,kSecClass,kSecClassGenericPassword);
 CFDictionarySetValue(q,kSecAttrService,CFSTR("steward"));
 CFDictionarySetValue(q,kSecAttrAccount,account); CFRelease(account);
 return q;
}
static OSStatus steward_get(const char *ref, CFDataRef *data) {
 CFMutableDictionaryRef q=steward_query(ref);
 CFDictionarySetValue(q,kSecReturnData,kCFBooleanTrue);
 OSStatus status=SecItemCopyMatching(q,(CFTypeRef*)data); CFRelease(q); return status;
}
static OSStatus steward_put(const char *ref, const void *bytes, CFIndex length) {
 CFMutableDictionaryRef q=steward_query(ref);
 CFDataRef data=CFDataCreate(NULL,bytes,length);
 const void *keys[]={kSecValueData}; const void *values[]={data};
 CFDictionaryRef changes=CFDictionaryCreate(NULL,keys,values,1,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 OSStatus status=SecItemUpdate(q,changes);
 if(status==errSecItemNotFound) { CFDictionarySetValue(q,kSecValueData,data); status=SecItemAdd(q,NULL); }
 CFRelease(changes); CFRelease(data); CFRelease(q); return status;
}
static OSStatus steward_remove(const char *ref) {
 CFMutableDictionaryRef q=steward_query(ref);
 OSStatus status=SecItemDelete(q); CFRelease(q);
 return status==errSecItemNotFound ? errSecSuccess : status;
}
*/
import "C"
import (
	"context"
	"unsafe"

	"github.com/z-chenhao/assistant/internal/llm"
)

func get(ctx context.Context, ref string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, llm.ErrSecretStore
	}
	key := C.CString(ref)
	defer C.free(unsafe.Pointer(key))
	var data C.CFDataRef
	if C.steward_get(key, &data) != C.errSecSuccess {
		return nil, llm.ErrSecretStore
	}
	defer C.CFRelease(C.CFTypeRef(data))
	return C.GoBytes(unsafe.Pointer(C.CFDataGetBytePtr(data)), C.int(C.CFDataGetLength(data))), nil
}
func put(ctx context.Context, ref string, value []byte) error {
	if ctx.Err() != nil || len(value) == 0 {
		return llm.ErrSecretStore
	}
	key := C.CString(ref)
	defer C.free(unsafe.Pointer(key))
	if C.steward_put(key, unsafe.Pointer(&value[0]), C.CFIndex(len(value))) != C.errSecSuccess {
		return llm.ErrSecretStore
	}
	return nil
}
func remove(ctx context.Context, ref string) error {
	if ctx.Err() != nil {
		return llm.ErrSecretStore
	}
	key := C.CString(ref)
	defer C.free(unsafe.Pointer(key))
	if C.steward_remove(key) != C.errSecSuccess {
		return llm.ErrSecretStore
	}
	return nil
}
