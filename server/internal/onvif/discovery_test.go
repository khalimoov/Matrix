package onvif

import "testing"

func TestParseProbeMatches(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery">
<SOAP-ENV:Body><d:ProbeMatches><d:ProbeMatch>
<d:Scopes>onvif://www.onvif.org/type/video_encoder onvif://www.onvif.org/name/Front%20Door onvif://www.onvif.org/hardware/IPC-HDW2431T</d:Scopes>
<d:XAddrs>http://169.254.1.10/onvif/device_service http://192.168.1.64/onvif/device_service</d:XAddrs>
</d:ProbeMatch></d:ProbeMatches></SOAP-ENV:Body></SOAP-ENV:Envelope>`)
	got := parseProbeMatches(data, "192.168.1.64")
	if len(got) != 1 {
		t.Fatalf("expected 1 device, got %d", len(got))
	}
	d := got[0]
	if d.DeviceURL != "http://192.168.1.64/onvif/device_service" {
		t.Errorf("device URL: %s", d.DeviceURL)
	}
	if d.Name != "Front Door" || d.Hardware != "IPC-HDW2431T" {
		t.Errorf("scopes: %+v", d)
	}
}
