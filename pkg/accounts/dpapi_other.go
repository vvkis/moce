//go:build !windows

package accounts

import "fmt"

func protect(Account) (string, error) {
	return "", fmt.Errorf("当前系统不能用 Windows 用户加密，请改用明文保存")
}

func unprotect(string) (Account, error) {
	return Account{}, fmt.Errorf("当前系统不能解开 Windows 用户加密")
}
