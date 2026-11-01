//go:build windows

package creds

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	crypt32              = syscall.NewLazyDLL("crypt32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtectData = crypt32.NewProc("CryptProtectData")
	procCryptUnprotect   = crypt32.NewProc("CryptUnprotectData")
	procLocalFree        = kernel32.NewProc("LocalFree")
)

const cryptProtectUIForbidden = 0x1

func protect(plain []byte) ([]byte, error) {
	in := dataBlob{cbData: uint32(len(plain))}
	if len(plain) > 0 {
		in.pbData = &plain[0]
	}
	var out dataBlob
	if hr, _, _ := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0,
		cryptProtectUIForbidden, uintptr(unsafe.Pointer(&out))); hr == 0 {
		return nil, errors.New("creds: CryptProtectData failed")
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	if out.cbData == 0 {
		return []byte{}, nil
	}
	return append([]byte(nil), unsafe.Slice(out.pbData, out.cbData)...), nil
}

func unprotect(cipher []byte) ([]byte, error) {
	if len(cipher) == 0 {
		return nil, nil
	}
	in := dataBlob{cbData: uint32(len(cipher)), pbData: &cipher[0]}
	var out dataBlob
	if hr, _, _ := procCryptUnprotect.Call(
		uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0,
		cryptProtectUIForbidden, uintptr(unsafe.Pointer(&out))); hr == 0 {
		return nil, fmt.Errorf("creds: CryptUnprotectData failed")
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return append([]byte(nil), unsafe.Slice(out.pbData, out.cbData)...), nil
}
