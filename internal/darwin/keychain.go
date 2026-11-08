//go:build darwin

package darwin

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// ErrKeychainItemNotFound reports that the queried item does not exist.
var ErrKeychainItemNotFound = errors.New("keychain: item not found")

// The generic-password item lives in the file-based login Keychain, the
// service and account as keys, mygo's secrets module does the same. The data
// protection keychain needs a signed app with keychain-access-groups, so it
// is not an option for this tree's unsigned builds.
const (
	errSecItemNotFound  = -25300
	errSecDuplicateItem = -25299
)

type keychainAPI struct {
	add, copy, update, delete, message uintptr
	class, genericPassword             ID
	service, account, label            ID
	valueData, returnData              ID
	matchLimit, matchLimitOne, cfTrue  ID
}

var keychain = sync.OnceValues(func() (*keychainAPI, error) {
	if err := load(); err != nil {
		return nil, err
	}
	lib, err := mustDlopen("/System/Library/Frameworks/Security.framework/Security")
	if err != nil {
		return nil, fmt.Errorf("keychain: %w", err)
	}
	k := &keychainAPI{}
	for _, e := range []struct {
		fn *uintptr
		nm string
	}{
		{&k.add, "SecItemAdd"},
		{&k.copy, "SecItemCopyMatching"},
		{&k.update, "SecItemUpdate"},
		{&k.delete, "SecItemDelete"},
		{&k.message, "SecCopyErrorMessageString"},
	} {
		if *e.fn, err = purego.Dlsym(lib, e.nm); err != nil {
			return nil, fmt.Errorf("keychain: %w", err)
		}
	}
	k.class = ConstString(lib, "kSecClass")
	k.genericPassword = ConstString(lib, "kSecClassGenericPassword")
	k.service = ConstString(lib, "kSecAttrService")
	k.account = ConstString(lib, "kSecAttrAccount")
	k.label = ConstString(lib, "kSecAttrLabel")
	k.valueData = ConstString(lib, "kSecValueData")
	k.returnData = ConstString(lib, "kSecReturnData")
	k.matchLimit = ConstString(lib, "kSecMatchLimit")
	k.matchLimitOne = ConstString(lib, "kSecMatchLimitOne")
	k.cfTrue = ConstString(libCF, "kCFBooleanTrue")
	for _, c := range []ID{k.class, k.genericPassword, k.service, k.account, k.valueData, k.returnData, k.matchLimit, k.matchLimitOne, k.cfTrue} {
		if c == 0 {
			return nil, errors.New("keychain: a kSec constant is missing")
		}
	}
	return k, nil
})

// query returns an autoreleased dictionary that finds the item of service
// and account.
func (k *keychainAPI) query(service, account string) ID {
	q := Send(mustClass("NSMutableDictionary"), "dictionary")
	set := func(key, v ID) { Send(q, "setObject:forKey:", uintptr(v), uintptr(key)) }
	set(k.class, k.genericPassword)
	set(k.service, NSString(service))
	set(k.account, NSString(account))
	return q
}

// status converts an OSStatus into an error with the system's message.
func (k *keychainAPI) status(op string, s int32) error {
	switch s {
	case 0:
		return nil
	case errSecItemNotFound:
		return ErrKeychainItemNotFound
	}
	msg := ""
	if str, _, _ := purego.SyscallN(k.message, uintptr(s), 0); str != 0 {
		msg = GoString(ID(str))
		CFRelease(str)
	}
	if msg == "" {
		msg = "unknown error"
	}
	return fmt.Errorf("keychain: %s: %s (OSStatus %d)", op, msg, s)
}

func osStatus(r uintptr) int32 { return int32(uint32(r)) }

// KeychainSet stores secret as a generic password of the login Keychain.
func KeychainSet(service, account string, secret []byte) error {
	k, err := keychain()
	if err != nil {
		return err
	}
	WithPool(func() {
		data := NSData(secret)
		update := Send(mustClass("NSMutableDictionary"), "dictionary")
		Send(update, "setObject:forKey:", uintptr(data), uintptr(k.valueData))
		r, _, _ := purego.SyscallN(k.update, uintptr(k.query(service, account)), uintptr(update))
		if s := osStatus(r); s != errSecItemNotFound {
			err = k.status("update", s)
			return
		}
		add := k.query(service, account)
		Send(add, "setObject:forKey:", uintptr(data), uintptr(k.valueData))
		Send(add, "setObject:forKey:", uintptr(NSString(service)), uintptr(k.label))
		r, _, _ = purego.SyscallN(k.add, uintptr(add), 0)
		if s := osStatus(r); s == errSecDuplicateItem {
			// Added by another thread or process since the update.
			r, _, _ = purego.SyscallN(k.update, uintptr(k.query(service, account)), uintptr(update))
			err = k.status("update", osStatus(r))
		} else {
			err = k.status("add", s)
		}
	})
	return err
}

// KeychainGet reads the generic password of service and account.
func KeychainGet(service, account string) (secret []byte, err error) {
	k, err := keychain()
	if err != nil {
		return nil, err
	}
	WithPool(func() {
		q := k.query(service, account)
		Send(q, "setObject:forKey:", uintptr(k.cfTrue), uintptr(k.returnData))
		Send(q, "setObject:forKey:", uintptr(k.matchLimitOne), uintptr(k.matchLimit))
		var data uintptr
		r, _, _ := purego.SyscallN(k.copy, uintptr(q), uintptr(unsafe.Pointer(&data)))
		if err = k.status("read", osStatus(r)); err != nil {
			return
		}
		if data == 0 {
			err = errors.New("keychain: read: no data")
			return
		}
		secret = GoBytes(ID(data))
		if secret == nil {
			secret = []byte{}
		}
		CFRelease(data)
	})
	return secret, err
}

// KeychainDelete removes the generic password of service and account. A
// missing item counts as success.
func KeychainDelete(service, account string) error {
	k, err := keychain()
	if err != nil {
		return err
	}
	WithPool(func() {
		r, _, _ := purego.SyscallN(k.delete, uintptr(k.query(service, account)))
		if err = k.status("delete", osStatus(r)); errors.Is(err, ErrKeychainItemNotFound) {
			err = nil
		}
	})
	return err
}
