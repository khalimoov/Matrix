package onvif

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
)

type DeviceInfo struct {
	Manufacturer    string `json:"manufacturer" xml:"Manufacturer"`
	Model           string `json:"model" xml:"Model"`
	FirmwareVersion string `json:"firmware_version" xml:"FirmwareVersion"`
	SerialNumber    string `json:"serial_number" xml:"SerialNumber"`
}

type Profile struct {
	Token     string `json:"token"`
	Name      string `json:"name"`
	Encoding  string `json:"encoding"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	HasPTZ    bool   `json:"has_ptz"`
	StreamURI string `json:"stream_uri"`
}

type services struct {
	media string
	ptz   string
}

func (c *Client) DeviceInformation(ctx context.Context) (DeviceInfo, error) {
	var info DeviceInfo
	err := c.call(ctx, c.DeviceURL, `<tds:GetDeviceInformation xmlns:tds="`+nsDevice+`"/>`, &info, true)
	return info, err
}

func (c *Client) services(ctx context.Context) (services, error) {
	var res struct {
		Media string `xml:"Capabilities>Media>XAddr"`
		PTZ   string `xml:"Capabilities>PTZ>XAddr"`
	}
	body := `<tds:GetCapabilities xmlns:tds="` + nsDevice + `"><tds:Category>All</tds:Category></tds:GetCapabilities>`
	if err := c.call(ctx, c.DeviceURL, body, &res, true); err != nil {
		return services{}, err
	}
	if res.Media == "" {
		return services{}, fmt.Errorf("onvif: device has no media service")
	}
	return services{media: c.reachable(res.Media), ptz: c.reachable(res.PTZ)}, nil
}

// reachable rewrites a service address reported by the device to use the host
// we already reached it on. Cameras behind NAT often report internal addresses.
func (c *Client) reachable(addr string) string {
	if addr == "" {
		return ""
	}
	svc, err1 := url.Parse(addr)
	dev, err2 := url.Parse(c.DeviceURL)
	if err1 != nil || err2 != nil {
		return addr
	}
	svc.Scheme, svc.Host = dev.Scheme, dev.Host
	return svc.String()
}

// Profiles returns the media profiles of the device with their RTSP stream URIs.
func (c *Client) Profiles(ctx context.Context) ([]Profile, error) {
	svc, err := c.services(ctx)
	if err != nil {
		return nil, err
	}
	var res struct {
		Profiles []struct {
			Token   string `xml:"token,attr"`
			Name    string `xml:"Name"`
			Encoder *struct {
				Encoding   string `xml:"Encoding"`
				Resolution struct {
					Width  int `xml:"Width"`
					Height int `xml:"Height"`
				} `xml:"Resolution"`
			} `xml:"VideoEncoderConfiguration"`
			PTZ *struct{} `xml:"PTZConfiguration"`
		} `xml:"Profiles"`
	}
	if err := c.call(ctx, svc.media, `<trt:GetProfiles xmlns:trt="`+nsMedia+`"/>`, &res, true); err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(res.Profiles))
	for _, p := range res.Profiles {
		pr := Profile{Token: p.Token, Name: p.Name, HasPTZ: p.PTZ != nil && svc.ptz != ""}
		if p.Encoder != nil {
			pr.Encoding = p.Encoder.Encoding
			pr.Width, pr.Height = p.Encoder.Resolution.Width, p.Encoder.Resolution.Height
		}
		if pr.StreamURI, err = c.streamURI(ctx, svc.media, p.Token); err != nil {
			return nil, err
		}
		profiles = append(profiles, pr)
	}
	return profiles, nil
}

func (c *Client) streamURI(ctx context.Context, mediaURL, token string) (string, error) {
	var res struct {
		URI string `xml:"MediaUri>Uri"`
	}
	body := `<trt:GetStreamUri xmlns:trt="` + nsMedia + `" xmlns:tt="` + nsSchema + `">
		<trt:StreamSetup><tt:Stream>RTP-Unicast</tt:Stream><tt:Transport><tt:Protocol>RTSP</tt:Protocol></tt:Transport></trt:StreamSetup>
		<trt:ProfileToken>` + html.EscapeString(token) + `</trt:ProfileToken></trt:GetStreamUri>`
	if err := c.call(ctx, mediaURL, body, &res, true); err != nil {
		return "", err
	}
	// Point the stream at the host we reach the device on, keeping the RTSP port.
	u, err := url.Parse(res.URI)
	dev, err2 := url.Parse(c.DeviceURL)
	if err != nil || err2 != nil || u.Host == "" {
		return res.URI, nil
	}
	port := u.Port()
	u.Host = dev.Hostname()
	if port != "" {
		u.Host += ":" + port
	}
	return u.String(), nil
}

// PTZ commands. Speeds are in the range -1..1.

func (c *Client) ptzURL(ctx context.Context) (string, error) {
	svc, err := c.services(ctx)
	if err != nil {
		return "", err
	}
	if svc.ptz == "" {
		return "", fmt.Errorf("onvif: device has no PTZ service")
	}
	return svc.ptz, nil
}

func (c *Client) ContinuousMove(ctx context.Context, profile string, pan, tilt, zoom float64) error {
	addr, err := c.ptzURL(ctx)
	if err != nil {
		return err
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }
	body := `<tptz:ContinuousMove xmlns:tptz="` + nsPTZ + `" xmlns:tt="` + nsSchema + `">
		<tptz:ProfileToken>` + html.EscapeString(profile) + `</tptz:ProfileToken>
		<tptz:Velocity><tt:PanTilt x="` + f(pan) + `" y="` + f(tilt) + `"/><tt:Zoom x="` + f(zoom) + `"/></tptz:Velocity>
		</tptz:ContinuousMove>`
	return c.call(ctx, addr, body, nil, true)
}

func (c *Client) Stop(ctx context.Context, profile string) error {
	addr, err := c.ptzURL(ctx)
	if err != nil {
		return err
	}
	body := `<tptz:Stop xmlns:tptz="` + nsPTZ + `"><tptz:ProfileToken>` + html.EscapeString(profile) +
		`</tptz:ProfileToken><tptz:PanTilt>true</tptz:PanTilt><tptz:Zoom>true</tptz:Zoom></tptz:Stop>`
	return c.call(ctx, addr, body, nil, true)
}

type Preset struct {
	Token string `json:"token" xml:"token,attr"`
	Name  string `json:"name" xml:"Name"`
}

func (c *Client) Presets(ctx context.Context, profile string) ([]Preset, error) {
	addr, err := c.ptzURL(ctx)
	if err != nil {
		return nil, err
	}
	var res struct {
		Presets []Preset `xml:"Preset"`
	}
	body := `<tptz:GetPresets xmlns:tptz="` + nsPTZ + `"><tptz:ProfileToken>` + html.EscapeString(profile) +
		`</tptz:ProfileToken></tptz:GetPresets>`
	if err := c.call(ctx, addr, body, &res, true); err != nil {
		return nil, err
	}
	if res.Presets == nil {
		res.Presets = []Preset{}
	}
	return res.Presets, nil
}

func (c *Client) GotoPreset(ctx context.Context, profile, preset string) error {
	addr, err := c.ptzURL(ctx)
	if err != nil {
		return err
	}
	body := `<tptz:GotoPreset xmlns:tptz="` + nsPTZ + `"><tptz:ProfileToken>` + html.EscapeString(profile) +
		`</tptz:ProfileToken><tptz:PresetToken>` + html.EscapeString(preset) + `</tptz:PresetToken></tptz:GotoPreset>`
	return c.call(ctx, addr, body, nil, true)
}
