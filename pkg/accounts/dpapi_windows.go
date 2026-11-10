//go:build windows

package accounts

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func protect(acc Account) (string, error) {
	body, err := json.Marshal(acc)
	if err != nil {
		return "", err
	}
	var in, out windows.DataBlob
	in.Size = uint32(len(body))
	in.Data = &body[0]
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return "", fmt.Errorf("当前系统不能用 Windows 用户加密，请改用明文保存")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	raw := unsafe.Slice(out.Data, out.Size)
	buf := make([]byte, len(raw))
	copy(buf, raw)
	return base64.StdEncoding.EncodeToString(buf), nil
}

func unprotect(blob string) (Account, error) {
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil || len(raw) == 0 {
		return Account{}, fmt.Errorf("当前系统不能解开 Windows 用户加密")
	}
	var in, out windows.DataBlob
	in.Size = uint32(len(raw))
	in.Data = &raw[0]
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return Account{}, fmt.Errorf("当前系统不能解开 Windows 用户加密")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	plain := append([]byte(nil), unsafe.Slice(out.Data, out.Size)...)
	var acc Account
	if err := json.Unmarshal(plain, &acc); err != nil {
		return Account{}, err
	}
	return acc, nil
}
