package web

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tariffCalculator/static"
	"tariffCalculator/store"
)

// Web push (ADR 0008 Decision 3): the app's own VAPID keys (RFC 8292), and
// each message encrypted for its browser (RFC 8291, aes128gcm), on the
// standard library alone.

// errGone is a push subscription the push service says no longer exists.
var errGone = errors.New("push subscription gone")

// pushServices are the push services' hosts a browser can give an address
// at; anything else is refused, so the server only ever posts to them.
var pushServices = []string{
	"fcm.googleapis.com", "android.googleapis.com", // Chrome, Edge on Android
	"updates.push.services.mozilla.com", "push.services.mozilla.com", // Firefox
	".push.apple.com",     // Safari, and iPhone home screen apps
	".notify.windows.com", // Edge on Windows
}

// pushEndpoint says whether a push address is one the server will post to.
func pushEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	for _, s := range pushServices {
		if u.Hostname() == s || strings.HasPrefix(s, ".") && strings.HasSuffix(u.Hostname(), s) {
			return true
		}
	}
	return false
}

// pushKeys are a browser's keys for its push subscription, as it gives them
// (base64url): its public key on P-256 and an authentication secret.
type pushKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// pushMessage is what the service worker shows (static/js/sw.js).
type pushMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// vapid is the app's key pair for identifying itself to push services.
type vapid struct {
	key    *ecdsa.PrivateKey
	public string // base64url, uncompressed, for browsers' applicationServerKey
}

// vapidKeys are the app's VAPID keys, made the first time and kept.
func (n *notifier) vapidKeys(ctx context.Context) (*vapid, error) {
	if n.keys != nil {
		return n.keys, nil
	}
	raw, err := n.st.Setting(ctx, "vapid-private-key", func() (string, error) {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return "", err
		}
		b, err := k.Bytes()
		return base64.RawURLEncoding.EncodeToString(b), err
	})
	if err != nil {
		return nil, err
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	k, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), b)
	if err != nil {
		return nil, err
	}
	pub, err := k.PublicKey.Bytes()
	if err != nil {
		return nil, err
	}
	n.keys = &vapid{key: k, public: base64.RawURLEncoding.EncodeToString(pub)}
	return n.keys, nil
}

// pushPublicKey is the key a browser subscribes with, "" if it can't be had.
func (n *notifier) pushPublicKey(ctx context.Context) string {
	if n == nil {
		return ""
	}
	v, err := n.vapidKeys(ctx)
	if err != nil {
		return ""
	}
	return v.public
}

// push sends one message to a push subscription.
func (n *notifier) push(ctx context.Context, sub store.Subscription, m pushMessage) error {
	if !pushEndpoint(sub.Address) {
		return errGone
	}
	var keys pushKeys
	if err := json.Unmarshal([]byte(sub.Keys), &keys); err != nil {
		return errGone
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}
	body, err := encryptPush(keys, payload)
	if err != nil {
		return errGone // keys a browser can't have made
	}
	v, err := n.vapidKeys(ctx)
	if err != nil {
		return err
	}
	u, _ := url.Parse(sub.Address)
	token, err := vapidToken(v.key, u.Scheme+"://"+u.Host, n.base, n.now())
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Address, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", "vapid t="+token+", k="+v.public)
	res, err := n.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return errGone
	case res.StatusCode >= 300:
		return fmt.Errorf("push service: %s", res.Status)
	}
	return nil
}

// client is the HTTP client pushes go by: no redirects followed, a short
// timeout.
func (n *notifier) client() *http.Client {
	if n.http != nil {
		return n.http
	}
	return &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// vapidToken is the signed JWT (ES256) that says who's pushing (RFC 8292):
// for the push service's origin, valid for 12 hours, with the site as
// contact.
func vapidToken(key *ecdsa.PrivateKey, audience, subject string, now time.Time) (string, error) {
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, err := json.Marshal(map[string]any{"aud": audience, "exp": now.Add(12 * time.Hour).Unix(), "sub": subject})
	if err != nil {
		return "", err
	}
	signing := header + "." + enc.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + enc.EncodeToString(sig), nil
}

// verifyVapidToken checks a token's signature, for tests.
func verifyVapidToken(pub *ecdsa.PublicKey, token string) bool {
	i := strings.LastIndex(token, ".")
	if i < 0 {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(token[i+1:])
	if err != nil || len(sig) != 64 {
		return false
	}
	digest := sha256.Sum256([]byte(token[:i]))
	return ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:]))
}

// handleServiceWorker serves the service worker at the site's root, so it
// covers every page.
func handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(static.Bytes("js/sw.js"))
}

// handleManifest is the web manifest that lets the competition pages be
// added to a phone's home screen, which iPhones need for push. It has no
// start_url, so the home screen opens the page it was added from: the
// person's own secret link.
func handleManifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(`{"name":"Trampoline competitions","short_name":"Competitions","display":"standalone","scope":"/","background_color":"#ffffff","theme_color":"#00d1b2"}`))
}

// pushRecordSize is the one record's size, as the header says (RFC 8188).
const pushRecordSize = 4096

// encryptPush encrypts a message for a browser's keys (RFC 8291): a key
// agreed between a new key pair and the browser's, mixed with its
// authentication secret, seals the message in a single aes128gcm record.
func encryptPush(keys pushKeys, payload []byte) ([]byte, error) {
	uaPublic, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(keys.P256dh, "="))
	if err != nil {
		return nil, err
	}
	authSecret, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(keys.Auth, "="))
	if err != nil || len(authSecret) != 16 {
		return nil, errors.New("bad auth secret")
	}
	ua, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, err
	}
	as, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	return sealPush(as, ua, authSecret, salt, payload)
}

// sealPush is encryptPush with its key pair and salt given.
func sealPush(as *ecdh.PrivateKey, ua *ecdh.PublicKey, authSecret, salt, payload []byte) ([]byte, error) {
	if len(payload)+1+16 > pushRecordSize-86 {
		return nil, errors.New("push message too long")
	}
	shared, err := as.ECDH(ua)
	if err != nil {
		return nil, err
	}
	asPublic, uaPublic := as.PublicKey().Bytes(), ua.Bytes()
	cek, nonce, err := pushKeysFor(shared, authSecret, salt, uaPublic, asPublic)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, append(append([]byte{}, payload...), 2), nil) // 2: the last record, no padding
	header := make([]byte, 0, 21+len(asPublic))
	header = append(header, salt...)
	header = binary.BigEndian.AppendUint32(header, pushRecordSize)
	header = append(header, byte(len(asPublic)))
	header = append(header, asPublic...)
	return append(header, sealed...), nil
}

// pushKeysFor derives a message's content key and nonce (RFC 8291 section 3.4).
func pushKeysFor(shared, authSecret, salt, uaPublic, asPublic []byte) (cek, nonce []byte, err error) {
	prkKey, err := hkdf.Extract(sha256.New, shared, authSecret)
	if err != nil {
		return nil, nil, err
	}
	keyInfo := "WebPush: info\x00" + string(uaPublic) + string(asPublic)
	ikm, err := hkdf.Expand(sha256.New, prkKey, keyInfo, 32)
	if err != nil {
		return nil, nil, err
	}
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, nil, err
	}
	if cek, err = hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16); err != nil {
		return nil, nil, err
	}
	nonce, err = hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	return cek, nonce, err
}
