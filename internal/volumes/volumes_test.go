package volumes

import (
	"testing"

	"github.com/callmemhz/milo/pkg/api"
)

func TestParseString_OK(t *testing.T) {
	tests := []struct {
		in   string
		want api.VolumeSpec
	}{
		{
			in:   "cpfs://train-fs/datasets/foo:/data",
			want: api.VolumeSpec{Source: "cpfs://train-fs/datasets/foo", Target: "/data"},
		},
		{
			in:   "cpfs://train-fs/datasets/foo:/data:ro",
			want: api.VolumeSpec{Source: "cpfs://train-fs/datasets/foo", Target: "/data", ReadOnly: true},
		},
		{
			in:   "cpfs://train-fs:/data",
			want: api.VolumeSpec{Source: "cpfs://train-fs", Target: "/data"},
		},
		{
			in:   "cpfs://train-fs/:/mnt/x:rw",
			want: api.VolumeSpec{Source: "cpfs://train-fs/", Target: "/mnt/x", ReadOnly: false},
		},
	}
	for _, tt := range tests {
		got, err := ParseString(tt.in)
		if err != nil {
			t.Errorf("ParseString(%q) unexpected err: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseString(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseString_Errors(t *testing.T) {
	bad := []string{
		"",
		"cpfs://train-fs",                 // no target
		"train-fs/datasets/foo:/data",     // no scheme
		"cpfs://train-fs/sub:/data:weird", // unknown opt
		"cpfs://train-fs/sub:relative",    // non-absolute target
		"cpfs://train-fs/sub:/data/../..", // dotdot in target
		"cpfs:///just-a-path:/data",       // missing fs-id
		"cpfs://../bad:/data",             // bad fs-id
		"cpfs://fs/../escape:/data",       // dotdot in sub-path
		"oss://bucket/path:/data",         // unsupported scheme (phase 1)
	}
	for _, s := range bad {
		if _, err := ParseString(s); err == nil {
			t.Errorf("ParseString(%q) expected error, got nil", s)
		}
	}
}

func TestHostPath(t *testing.T) {
	tests := []struct {
		in   api.VolumeSpec
		want string
	}{
		{api.VolumeSpec{Source: "cpfs://train-fs/datasets/foo"}, "/mnt/cpfs/train-fs/datasets/foo"},
		{api.VolumeSpec{Source: "cpfs://train-fs"}, "/mnt/cpfs/train-fs"},
		{api.VolumeSpec{Source: "cpfs://train-fs/"}, "/mnt/cpfs/train-fs"},
	}
	for _, tt := range tests {
		got, err := HostPath(tt.in)
		if err != nil {
			t.Errorf("HostPath(%+v) err: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("HostPath(%+v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
