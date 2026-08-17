package sign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const VersionV1 = "v1"
const VersionV2 = "v2"

func HMACSHA256(clientID, secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(clientID + timestamp))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func BodyHash(body []byte) string {
	if body == nil {
		body = []byte{}
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func CanonicalQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	v, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		vs := append([]string{}, v[k]...)
		sort.Strings(vs)
		for _, x := range vs {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(x))
		}
	}
	return strings.Join(parts, "&")
}

func CanonicalV2(clientID, method, path, rawQuery, tenant, timestamp, nonce string, body []byte) string {
	return strings.Join([]string{
		VersionV2,
		clientID,
		strings.ToUpper(method),
		path,
		CanonicalQuery(rawQuery),
		tenant,
		timestamp,
		nonce,
		BodyHash(body),
	}, "\n")
}

func HMACV2(secret, clientID, method, path, rawQuery, tenant, timestamp, nonce string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(CanonicalV2(clientID, method, path, rawQuery, tenant, timestamp, nonce, body)))
	return hex.EncodeToString(mac.Sum(nil))
}

func Timestamp() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}
