// Derived from moon (https://github.com/AnkioTomas/moon), GNU GPLv3.

package accounts

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

type qrSession struct {
	uuid     string
	deviceID string
	deadline time.Time
}

var (
	qrMu       sync.Mutex
	qrSessions = map[string]*qrSession{}
)

// StartWeReadQR asks WeRead for a WeChat login code and returns a PNG.
func StartWeReadQR(ctx context.Context) (string, string, error) {
	body, code, err := get(ctx, "https://i.weread.qq.com/wxticket?nonceStr=weread", "", map[string]string{
		"User-Agent": "WeRead/10.2.2",
	})
	if err != nil || code != 200 {
		return "", "", fmt.Errorf("获取二维码票据失败")
	}
	var ticket struct {
		Signature string          `json:"signature"`
		TimeStamp json.RawMessage `json:"timeStamp"`
	}
	if json.Unmarshal([]byte(body), &ticket) != nil || ticket.Signature == "" {
		return "", "", fmt.Errorf("获取二维码票据失败")
	}
	q := url.Values{}
	q.Set("appid", "wxab9b71ad2b90ff34")
	q.Set("noncestr", "weread")
	q.Set("timestamp", string(bytes.Trim(ticket.TimeStamp, `"`)))
	q.Set("scope", "snsapi_userinfo,snsapi_timeline,snsapi_friend")
	q.Set("signature", ticket.Signature)
	body, code, err = get(ctx, "https://open.weixin.qq.com/connect/sdk/qrconnect?"+q.Encode(), "", nil)
	if err != nil || code != 200 {
		return "", "", fmt.Errorf("获取二维码失败")
	}
	var qr struct {
		ErrCode int    `json:"errcode"`
		UUID    string `json:"uuid"`
	}
	if json.Unmarshal([]byte(body), &qr) != nil || qr.ErrCode != 0 || qr.UUID == "" {
		return "", "", fmt.Errorf("获取二维码失败")
	}
	payload := "https://open.weixin.qq.com/connect/confirm?uuid=" + url.QueryEscape(qr.UUID)
	png, err := qrcode.Encode(payload, qrcode.Medium, 256)
	if err != nil {
		return "", "", fmt.Errorf("获取二维码失败")
	}
	id := newSession()
	qrMu.Lock()
	qrSessions[id] = &qrSession{uuid: qr.UUID, deviceID: newDevice(), deadline: time.Now().Add(5 * time.Minute)}
	qrMu.Unlock()
	return id, base64.StdEncoding.EncodeToString(png), nil
}

// PollWeReadQR continues a scan. done is true when the account file has been written.
func PollWeReadQR(ctx context.Context, id string) (bool, error) {
	qrMu.Lock()
	sess := qrSessions[id]
	qrMu.Unlock()
	if sess == nil || time.Now().After(sess.deadline) {
		return false, fmt.Errorf("二维码已失效，请重新登录")
	}
	rawURL := "https://long.open.weixin.qq.com/connect/l/qrconnect?f=json&uuid=" + url.QueryEscape(sess.uuid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, fmt.Errorf("登录失败")
	}
	client := &http.Client{Timeout: 70 * time.Second, Transport: Transport}
	res, err := client.Do(req)
	if err != nil {
		return false, nil
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var data struct {
		WxErrCode int    `json:"wx_errcode"`
		WxCode    string `json:"wx_code"`
	}
	_ = json.Unmarshal(body, &data)
	switch data.WxErrCode {
	case 404, 408, 0:
		return false, nil
	case 403:
		return false, fmt.Errorf("已在微信中拒绝登录")
	case 405:
		if data.WxCode == "" {
			return false, fmt.Errorf("登录失败")
		}
		return finishWeRead(ctx, sess, data.WxCode)
	default:
		return false, fmt.Errorf("二维码已失效，请重新登录")
	}
}

func finishWeRead(ctx context.Context, sess *qrSession, wxCode string) (bool, error) {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	n := mrand.Intn(1000)
	sum := sha256.Sum256([]byte(ts + sess.deviceID + strconv.Itoa(n)))
	payload := map[string]any{
		"code": wxCode, "deviceId": sess.deviceID, "deviceName": "Xiaomi",
		"isFromQrcode": 1, "random": n, "timestamp": ts,
		"signature": hex.EncodeToString(sum[:]), "deviceType": 1,
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://i.weread.qq.com/login", bytes.NewReader(raw))
	if err != nil {
		return false, fmt.Errorf("登录失败")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "WeRead/10.2.2")
	client := &http.Client{Timeout: 20 * time.Second, Transport: Transport}
	res, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("登录失败")
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var data struct {
		Vid         any    `json:"vid"`
		AccessToken string `json:"accessToken"`
	}
	if json.Unmarshal(body, &data) != nil || data.AccessToken == "" || data.Vid == nil {
		return false, fmt.Errorf("登录失败")
	}
	vid := fmt.Sprint(data.Vid)
	acc := Account{AccountID: vid, Token: data.AccessToken, Name: vid, Cookie: "wr_vid=" + vid + "; wr_skey=" + data.AccessToken}
	if err := Commit(ctx, "weread", acc.Cookie); err != nil {
		// The scan already proved the session. Save directly if the profile check is unreachable.
		if err := Put("weread", acc); err != nil {
			return false, err
		}
	}
	return true, nil
}

func newSession() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func newDevice() string {
	return strconv.Itoa(mrand.Intn(9)+1) + strconv.FormatInt(time.Now().UnixNano(), 10)
}
