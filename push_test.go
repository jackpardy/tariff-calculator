package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// openPush decrypts a push message as a browser does (RFC 8291).
func openPush(t *testing.T, ua *ecdh.PrivateKey, auth, body []byte) []byte {
	t.Helper()
	if len(body) < 21 || binary.BigEndian.Uint32(body[16:20]) != pushRecordSize {
		t.Fatal("a header with the record size")
	}
	salt, idlen := body[:16], int(body[20])
	asPublic := body[21 : 21+idlen]
	as, err := ecdh.P256().NewPublicKey(asPublic)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := ua.ECDH(as)
	if err != nil {
		t.Fatal(err)
	}
	cek, nonce, err := pushKeysFor(shared, auth, salt, ua.PublicKey().Bytes(), asPublic)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, body[21+idlen:], nil)
	if err != nil {
		t.Fatalf("decrypting: %v", err)
	}
	if plain[len(plain)-1] != 2 {
		t.Fatal("the last record's delimiter")
	}
	return plain[:len(plain)-1]
}

// RFC 8291's example (section 5): the same keys and salt give its message.
func TestPushEncryptionExample(t *testing.T) {
	d := func(s string) []byte {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	as, _ := ecdh.P256().NewPrivateKey(d("yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	ua, _ := ecdh.P256().NewPublicKey(d("BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"))
	got, err := sealPush(as, ua, d("BTBZMqHH6r4Tts7J_aSIgg"), d("DGv6ra1nlYgDCS1FRnbzlw"), []byte("When I grow up, I want to be a watermelon"))
	if err != nil {
		t.Fatal(err)
	}
	want := d("DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN")
	if !bytes.Equal(got, want) {
		t.Errorf("the example:\n%x\n%x", got, want)
	}
}

func TestPushEndpoints(t *testing.T) {
	for endpoint, ok := range map[string]bool{
		"https://fcm.googleapis.com/fcm/send/abc":                true,
		"https://web.push.apple.com/QGuQyavXutnMH":               true,
		"https://updates.push.services.mozilla.com/wpush/v2/abc": true,
		"https://wns2-par02p.notify.windows.com/w/?token=abc":    true,
		"http://fcm.googleapis.com/fcm/send/abc":                 false,
		"https://evil.example/push.apple.com":                    false,
		"https://notpush.apple.com.evil.example/x":               false,
		"https://fcm.googleapis.com:8443/x":                      false,
		"https://localhost/x":                                    false,
	} {
		if pushEndpoint(endpoint) != ok {
			t.Errorf("%s: want %v", endpoint, ok)
		}
	}
}

// fakePushService takes what's pushed instead of the network.
type fakePushService struct {
	got    []*http.Request
	bodies [][]byte
	status int
}

func (f *fakePushService) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	f.got, f.bodies = append(f.got, r), append(f.bodies, body)
	return &http.Response{StatusCode: f.status, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
}

func TestPushNotifications(t *testing.T) {
	h, p, _ := notifyServer(t)
	service := &fakePushService{status: http.StatusCreated}
	p.notify.http = &http.Client{Transport: service}
	admin := created(t, h, newCompetition())
	enter := pathIn(t, do(t, h, http.MethodGet, admin, nil).Body.String(), "/competitions/enter/")
	own := withoutQuery(redirected(t, h, enter, url.Values{"gymnast": {"Ann Ryan"}, "level": {"BUCS L3"}, "ex1Option": {"builtin:bucs-l3-option-1"}, "ex2Option": {"builtin:bucs-l3-second"}, "ex2Skills": {voluntary}}))
	notify := own + "/notify"
	if page := do(t, h, http.MethodGet, notify, nil).Body.String(); !strings.Contains(page, `data-push-key="B`) {
		t.Fatal("the page has the server's push key")
	}

	// The phone's keys, as a browser makes them.
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)
	phone := url.Values{"action": {"push"}, "endpoint": {"https://fcm.googleapis.com/fcm/send/ann"},
		"p256dh": {base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())}, "auth": {base64.RawURLEncoding.EncodeToString(auth)}, "topic": {"cards"}}
	evil := url.Values{}
	for k, v := range phone {
		evil[k] = v
	}
	evil.Set("endpoint", "https://169.254.169.254/latest")
	if rec := do(t, h, http.MethodPost, notify, evil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("only push services: %d", rec.Code)
	}
	redirected(t, h, notify, phone)
	if page := do(t, h, http.MethodGet, notify, nil).Body.String(); !strings.Contains(page, "A phone or browser added") {
		t.Error("the phone is listed")
	}

	// A card checked: one push, encrypted for the phone, signed by the server.
	entry := entryLink(t, h, admin)
	redirected(t, h, entry+"/check", url.Values{"checked": {"1"}})
	redirected(t, h, admin+"/notify-now", nil)
	p.notify.tell(context.Background())
	if len(service.got) != 1 {
		t.Fatalf("one push: %d", len(service.got))
	}
	req := service.got[0]
	if req.URL.String() != "https://fcm.googleapis.com/fcm/send/ann" || req.Header.Get("Content-Encoding") != "aes128gcm" || req.Header.Get("TTL") == "" {
		t.Errorf("the request: %s %v", req.URL, req.Header)
	}
	var m pushMessage
	if err := json.Unmarshal(openPush(t, ua, auth, service.bodies[0]), &m); err != nil {
		t.Fatal(err)
	}
	if m.Title != "Student Open" || m.Body != "Ann Ryan · BUCS L3: card checked" || m.URL != "https://tariff.example"+own {
		t.Errorf("the message: %+v", m)
	}
	authz := req.Header.Get("Authorization")
	token, key, _ := strings.Cut(strings.TrimPrefix(authz, "vapid t="), ", k=")
	raw, _ := base64.RawURLEncoding.DecodeString(key)
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), raw)
	if err != nil || !verifyVapidToken(pub, token) {
		t.Errorf("signed with the server's key: %s", authz)
	}
	claims, _ := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
	if !strings.Contains(string(claims), `"aud":"https://fcm.googleapis.com"`) {
		t.Errorf("for the push service: %s", claims)
	}

	// The push service says the phone's gone: it's dropped.
	service.status = http.StatusGone
	redirected(t, h, entry+"/check", url.Values{"checked": {"1"}, "note": {"Fix element 7"}})
	redirected(t, h, admin+"/notify-now", nil)
	p.notify.tell(context.Background())
	if page := do(t, h, http.MethodGet, notify, nil).Body.String(); strings.Contains(page, "A phone or browser added") {
		t.Error("a gone phone is dropped")
	}
}

func TestServiceWorkerAndManifest(t *testing.T) {
	h := competitionServer(t)
	if rec := do(t, h, http.MethodGet, "/sw.js", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "showNotification") || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") {
		t.Errorf("the service worker: %d %s", rec.Code, rec.Header())
	}
	rec := do(t, h, http.MethodGet, "/manifest.webmanifest", nil)
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil || m["display"] != "standalone" || m["start_url"] != nil {
		t.Errorf("the manifest, opening the page it's added from: %s", rec.Body)
	}
}

// entryLink is the organiser's link to the only entry.
func entryLink(t *testing.T, h http.Handler, admin string) string {
	t.Helper()
	m := regexp.MustCompile(`href="(` + regexp.QuoteMeta(admin) + `/entries/[^"]+)"`).FindStringSubmatch(do(t, h, http.MethodGet, admin, nil).Body.String())
	if m == nil {
		t.Fatal("no entry")
	}
	return m[1]
}
