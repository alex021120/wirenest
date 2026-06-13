package wg

import (
	"encoding/base64"

	"github.com/skip2/go-qrcode"
)

// QRDataURL renders text as a QR code PNG and returns it as a data: URL, ready
// to drop into an <img src>. Used so phones can scan a client config to import.
func QRDataURL(text string) (string, error) {
	png, err := qrcode.Encode(text, qrcode.Medium, 320)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}
