package sign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
)

func HMACSHA256(clientID, secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(clientID + timestamp))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func Timestamp() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}
