package accounts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Account is one source's saved login. Cookies are never written to logs.
type Account struct {
	Cookie    string `json:"cookie,omitempty"`
	Token     string `json:"token,omitempty"`
	AccountID string `json:"accountId,omitempty"`
	Name      string `json:"name,omitempty"`
}

type envelope struct {
	Mode    string   `json:"mode"`
	Account *Account `json:"account,omitempty"`
	Blob    string   `json:"blob,omitempty"`
}

var (
	dataDir string
	modeOf  = func() string { return "plain" }
)

// Use points account files at <exe>/data and reads the plain/dpapi switch.
func Use(dir string, mode func() string) {
	dataDir = dir
	if mode != nil {
		modeOf = mode
	}
}

func dir() string {
	if dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, "accounts")
}

func path(id string) string {
	return filepath.Join(dir(), id+".json")
}

// Get loads one source. The second result is false when no file exists.
func Get(id string) (Account, bool, error) {
	if dir() == "" {
		return Account{}, false, nil
	}
	body, err := os.ReadFile(path(id))
	if os.IsNotExist(err) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, err
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return Account{}, false, err
	}
	if env.Mode == "dpapi" {
		acc, err := unprotect(env.Blob)
		if err != nil {
			return Account{}, false, err
		}
		return acc, true, nil
	}
	if env.Account == nil {
		return Account{}, true, nil
	}
	return *env.Account, true, nil
}

// Put writes one source atomically inside data/accounts.
func Put(id string, acc Account) error {
	folder := dir()
	if folder == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	mode := modeOf()
	if mode != "dpapi" {
		mode = "plain"
	}
	env := envelope{Mode: mode}
	if mode == "dpapi" {
		blob, err := protect(acc)
		if err != nil {
			return err
		}
		env.Blob = blob
	} else {
		env.Account = &acc
	}
	body, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	final := path(id)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// Delete removes the source file. Missing files are fine.
func Delete(id string) error {
	if dir() == "" {
		return nil
	}
	err := os.Remove(path(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func cleanID(id string) string {
	id = strings.TrimSpace(id)
	if strings.Contains(id, "/") || strings.Contains(id, `\`) || id == "" || id == "." || id == ".." {
		return ""
	}
	return id
}
