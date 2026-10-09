package store

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Vendor decides how RTSP addresses are built for a camera.
type Vendor string

const (
	VendorDahua     Vendor = "dahua"     // also Imou, Amcrest and other Dahua OEMs
	VendorHikvision Vendor = "hikvision" // also HiWatch and other Hikvision OEMs
	VendorONVIF     Vendor = "onvif"     // URLs discovered through ONVIF
	VendorGeneric   Vendor = "generic"   // URLs typed in by hand
)

func (v Vendor) Valid() bool {
	return v == VendorDahua || v == VendorHikvision || v == VendorONVIF || v == VendorGeneric
}

type Camera struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Group         string    `json:"group"`
	Vendor        Vendor    `json:"vendor"`
	Host          string    `json:"host"`
	RTSPPort      int       `json:"rtsp_port"`
	Username      string    `json:"username"`
	Password      string    `json:"-"`
	Channel       int       `json:"channel"`
	MainURL       string    `json:"main_url"`
	SubURL        string    `json:"sub_url"`
	ONVIFURL      string    `json:"onvif_url"`
	ONVIFProfile  string    `json:"onvif_profile"`
	Record        bool      `json:"record"`
	RetentionDays int       `json:"retention_days"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
}

// PathName is the MediaMTX path that carries the camera's main stream.
func (c Camera) PathName() string { return "cam" + strconv.FormatInt(c.ID, 10) }

// SubPathName is the MediaMTX path that carries the low-resolution sub stream.
func (c Camera) SubPathName() string { return c.PathName() + "_sub" }

// StreamURLs returns the RTSP addresses (with credentials) of the main and sub streams.
// sub is empty when the camera has no known sub stream.
func (c Camera) StreamURLs() (main, sub string, err error) {
	switch c.Vendor {
	case VendorDahua:
		path := "/cam/realmonitor?channel=%d&subtype=%d"
		main = c.rtspURL(fmt.Sprintf(path, c.Channel, 0))
		sub = c.rtspURL(fmt.Sprintf(path, c.Channel, 1))
	case VendorHikvision:
		main = c.rtspURL(fmt.Sprintf("/Streaming/Channels/%d01", c.Channel))
		sub = c.rtspURL(fmt.Sprintf("/Streaming/Channels/%d02", c.Channel))
	default:
		if main, err = c.withCredentials(c.MainURL); err != nil {
			return "", "", fmt.Errorf("основной поток: %w", err)
		}
		if c.SubURL != "" {
			if sub, err = c.withCredentials(c.SubURL); err != nil {
				return "", "", fmt.Errorf("дополнительный поток: %w", err)
			}
		}
	}
	return main, sub, nil
}

func (c Camera) rtspURL(pathAndQuery string) string {
	u := url.URL{Scheme: "rtsp", Host: net.JoinHostPort(c.Host, strconv.Itoa(c.RTSPPort))}
	if c.Username != "" {
		u.User = url.UserPassword(c.Username, c.Password)
	}
	return u.String() + pathAndQuery
}

// withCredentials adds the camera's login to a URL that has none.
func (c Camera) withCredentials(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme != "rtsp" && u.Scheme != "rtsps" {
		return "", fmt.Errorf("поддерживаются только адреса rtsp:// и rtsps://")
	}
	if u.User == nil && c.Username != "" {
		u.User = url.UserPassword(c.Username, c.Password)
	}
	return u.String(), nil
}

// Validate checks the fields a user can set and fills defaults.
func (c *Camera) Validate() error {
	c.Name = strings.TrimSpace(c.Name)
	c.Group = strings.TrimSpace(c.Group)
	c.Host = strings.TrimSpace(c.Host)
	if c.Name == "" {
		return fmt.Errorf("укажите название камеры")
	}
	if !c.Vendor.Valid() {
		return fmt.Errorf("неизвестный тип подключения %q", c.Vendor)
	}
	if c.RTSPPort == 0 {
		c.RTSPPort = 554
	}
	if c.Channel <= 0 {
		c.Channel = 1
	}
	if c.RetentionDays <= 0 {
		c.RetentionDays = 7
	}
	if c.RTSPPort < 1 || c.RTSPPort > 65535 {
		return fmt.Errorf("неверный RTSP-порт")
	}
	switch c.Vendor {
	case VendorDahua, VendorHikvision:
		if c.Host == "" {
			return fmt.Errorf("укажите IP-адрес камеры")
		}
	default:
		if c.MainURL == "" {
			return fmt.Errorf("укажите адрес основного потока")
		}
	}
	_, _, err := c.StreamURLs()
	return err
}

const cameraCols = `id, name, grp, vendor, host, rtsp_port, username, password, channel, main_url, sub_url,
	onvif_url, onvif_profile, record, retention_days, enabled, created_at`

func scanCamera(row interface{ Scan(...any) error }) (Camera, error) {
	var c Camera
	var record, enabled int
	var created int64
	err := row.Scan(&c.ID, &c.Name, &c.Group, &c.Vendor, &c.Host, &c.RTSPPort, &c.Username, &c.Password,
		&c.Channel, &c.MainURL, &c.SubURL, &c.ONVIFURL, &c.ONVIFProfile, &record, &c.RetentionDays, &enabled, &created)
	c.Record = record == 1
	c.Enabled = enabled == 1
	c.CreatedAt = time.Unix(created, 0)
	return c, notFound(err)
}

func (s *Store) ListCameras() ([]Camera, error) {
	rows, err := s.db.Query(`SELECT ` + cameraCols + ` FROM cameras ORDER BY grp, name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cams := []Camera{}
	for rows.Next() {
		c, err := scanCamera(rows)
		if err != nil {
			return nil, err
		}
		cams = append(cams, c)
	}
	return cams, rows.Err()
}

func (s *Store) Camera(id int64) (Camera, error) {
	return scanCamera(s.db.QueryRow(`SELECT `+cameraCols+` FROM cameras WHERE id = ?`, id))
}

func (s *Store) CreateCamera(c Camera) (Camera, error) {
	c.CreatedAt = time.Now()
	res, err := s.db.Exec(`INSERT INTO cameras (name, grp, vendor, host, rtsp_port, username, password, channel,
		main_url, sub_url, onvif_url, onvif_profile, record, retention_days, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Group, c.Vendor, c.Host, c.RTSPPort, c.Username, c.Password, c.Channel, c.MainURL, c.SubURL,
		c.ONVIFURL, c.ONVIFProfile, boolInt(c.Record), c.RetentionDays, boolInt(c.Enabled), c.CreatedAt.Unix())
	if err != nil {
		return Camera{}, err
	}
	c.ID, _ = res.LastInsertId()
	return c, nil
}

func (s *Store) UpdateCamera(c Camera) error {
	return s.execOne(`UPDATE cameras SET name = ?, grp = ?, vendor = ?, host = ?, rtsp_port = ?, username = ?,
		password = ?, channel = ?, main_url = ?, sub_url = ?, onvif_url = ?, onvif_profile = ?, record = ?,
		retention_days = ?, enabled = ? WHERE id = ?`,
		c.Name, c.Group, c.Vendor, c.Host, c.RTSPPort, c.Username, c.Password, c.Channel, c.MainURL, c.SubURL,
		c.ONVIFURL, c.ONVIFProfile, boolInt(c.Record), c.RetentionDays, boolInt(c.Enabled), c.ID)
}

func (s *Store) DeleteCamera(id int64) error {
	return s.execOne(`DELETE FROM cameras WHERE id = ?`, id)
}
