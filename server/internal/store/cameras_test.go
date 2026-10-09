package store

import "testing"

func TestStreamURLs(t *testing.T) {
	tests := []struct {
		name      string
		cam       Camera
		main, sub string
	}{
		{
			name: "dahua escapes credentials",
			cam:  Camera{Vendor: VendorDahua, Host: "10.0.0.5", RTSPPort: 554, Username: "admin", Password: "p@ss:w/rd", Channel: 2},
			main: "rtsp://admin:p%40ss%3Aw%2Frd@10.0.0.5:554/cam/realmonitor?channel=2&subtype=0",
			sub:  "rtsp://admin:p%40ss%3Aw%2Frd@10.0.0.5:554/cam/realmonitor?channel=2&subtype=1",
		},
		{
			name: "hikvision",
			cam:  Camera{Vendor: VendorHikvision, Host: "cam.local", RTSPPort: 10554, Channel: 1},
			main: "rtsp://cam.local:10554/Streaming/Channels/101",
			sub:  "rtsp://cam.local:10554/Streaming/Channels/102",
		},
		{
			name: "generic adds missing credentials",
			cam:  Camera{Vendor: VendorGeneric, Username: "u", Password: "p", MainURL: "rtsp://1.2.3.4/live"},
			main: "rtsp://u:p@1.2.3.4/live",
		},
		{
			name: "generic keeps credentials in URL",
			cam:  Camera{Vendor: VendorONVIF, Username: "u", Password: "p", MainURL: "rtsp://x:y@1.2.3.4/a", SubURL: "rtsp://1.2.3.4/b"},
			main: "rtsp://x:y@1.2.3.4/a",
			sub:  "rtsp://u:p@1.2.3.4/b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			main, sub, err := tt.cam.StreamURLs()
			if err != nil {
				t.Fatal(err)
			}
			if main != tt.main || sub != tt.sub {
				t.Errorf("got\n%s\n%s\nwant\n%s\n%s", main, sub, tt.main, tt.sub)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	bad := []Camera{
		{Vendor: VendorDahua, Host: "h"},                                // no name
		{Name: "a", Vendor: "sony", Host: "h"},                          // unknown vendor
		{Name: "a", Vendor: VendorDahua},                                // no host
		{Name: "a", Vendor: VendorGeneric},                              // no URL
		{Name: "a", Vendor: VendorGeneric, MainURL: "http://1.2.3.4/x"}, // not RTSP
		{Name: "a", Vendor: VendorDahua, Host: "h", RTSPPort: 70000},    // bad port
	}
	for i, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: expected an error", i)
		}
	}
	good := Camera{Name: " Gate ", Vendor: VendorDahua, Host: "h"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	if good.Name != "Gate" || good.RTSPPort != 554 || good.Channel != 1 || good.RetentionDays != 7 {
		t.Errorf("defaults not applied: %+v", good)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, err := s.CreateCamera(Camera{Name: "A", Vendor: VendorDahua, Host: "h", RTSPPort: 554, Channel: 1,
		Password: "secret", Record: true, RetentionDays: 3, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Camera(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Password != "secret" || !got.Record || got.RetentionDays != 3 {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if err := s.DeleteCamera(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Camera(c.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
