// Package onvif implements the parts of the ONVIF standard a VMS needs:
// WS-Discovery, device information, media profiles, stream URIs and PTZ.
package onvif

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	nsDevice = "http://www.onvif.org/ver10/device/wsdl"
	nsMedia  = "http://www.onvif.org/ver10/media/wsdl"
	nsPTZ    = "http://www.onvif.org/ver20/ptz/wsdl"
	nsSchema = "http://www.onvif.org/ver10/schema"
)

var httpClient = &http.Client{Timeout: 8 * time.Second}

// Client talks to one ONVIF device.
type Client struct {
	DeviceURL string // e.g. http://192.168.1.10/onvif/device_service
	Username  string
	Password  string

	once       sync.Once
	timeOffset time.Duration // device clock minus local clock, for WS-Security
}

func (c *Client) syncClock(ctx context.Context) {
	c.once.Do(func() {
		var res struct {
			UTC struct {
				Time struct{ Hour, Minute, Second int }
				Date struct{ Year, Month, Day int }
			} `xml:"SystemDateAndTime>UTCDateTime"`
		}
		body := `<tds:GetSystemDateAndTime xmlns:tds="` + nsDevice + `"/>`
		if err := c.call(ctx, c.DeviceURL, body, &res, false); err != nil || res.UTC.Date.Year == 0 {
			return
		}
		d, t := res.UTC.Date, res.UTC.Time
		device := time.Date(d.Year, time.Month(d.Month), d.Day, t.Hour, t.Minute, t.Second, 0, time.UTC)
		c.timeOffset = time.Until(device)
	})
}

// securityHeader builds a WS-Security UsernameToken with a password digest.
func (c *Client) securityHeader() string {
	if c.Username == "" {
		return ""
	}
	nonce := make([]byte, 16)
	rand.Read(nonce)
	created := time.Now().Add(c.timeOffset).UTC().Format("2006-01-02T15:04:05.000Z")
	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(c.Password))
	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return `<s:Header><wsse:Security s:mustUnderstand="1"
		xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
		xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
		<wsse:UsernameToken><wsse:Username>` + html.EscapeString(c.Username) + `</wsse:Username>
		<wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">` +
		digest + `</wsse:Password>
		<wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">` +
		base64.StdEncoding.EncodeToString(nonce) + `</wsse:Nonce>
		<wsu:Created>` + created + `</wsu:Created></wsse:UsernameToken></wsse:Security></s:Header>`
}

type soapFault struct {
	Code   string `xml:"Code>Subcode>Value"`
	Reason string `xml:"Reason>Text"`
}

// call sends a SOAP request and decodes the first element of the response body into out.
func (c *Client) call(ctx context.Context, url, body string, out any, auth bool) error {
	header := ""
	if auth {
		c.syncClock(ctx)
		header = c.securityHeader()
	}
	envelope := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` + header +
		`<s:Body>` + body + `</s:Body></s:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(envelope))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	var env struct {
		Body struct {
			Inner []byte     `xml:",innerxml"`
			Fault *soapFault `xml:"Fault"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("onvif: HTTP %d, invalid response: %w", resp.StatusCode, err)
	}
	if f := env.Body.Fault; f != nil {
		if strings.Contains(f.Code, "NotAuthorized") || resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("onvif: неверный логин или пароль")
		}
		return fmt.Errorf("onvif: %s %s", strings.TrimSpace(f.Code), strings.TrimSpace(f.Reason))
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("onvif: неверный логин или пароль")
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("onvif: HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return xml.NewDecoder(bytes.NewReader(env.Body.Inner)).Decode(out)
}
