package space

import (
	"math"
	"testing"

	"github.com/ozine/videohelper/internal/config"
)

func TestChannelsValidateConfigurationAndOwnership(t *testing.T) {
	valid := config.ChannelConfig{Doc: "video", Seg: "grid@v1", Weight: 1, Active: []string{"wemm-2b@r1"}}
	channels, err := NewChannels(config.ChannelsConfig{"visual": valid})
	if err != nil {
		t.Fatal(err)
	}
	valid.Active[0] = "changed"
	got, _ := channels.Get("visual")
	if got.Active[0] != "wemm-2b@r1" {
		t.Fatal("configuration mutates live channels")
	}
	got.Active[0] = "changed"
	if channels.List()[0].Active[0] != "wemm-2b@r1" {
		t.Fatal("caller mutates live channels")
	}
	valid.Active[0] = "wemm-2b@r1"
	for _, bad := range []config.ChannelConfig{
		{Doc: "audio", Seg: "grid@v1", Weight: 1, Active: valid.Active},
		{Doc: "video", Seg: "grid@v2", Weight: 1, Active: valid.Active},
		{Doc: "video", Seg: "grid@v1", Weight: math.NaN(), Active: valid.Active},
		{Doc: "video", Seg: "grid@v1", Weight: 1, Active: valid.Active, Shadow: valid.Active},
	} {
		if _, err := NewChannels(config.ChannelsConfig{"visual": bad}); err == nil {
			t.Fatalf("invalid channel accepted: %+v", bad)
		}
	}
	if _, err := NewChannels(config.ChannelsConfig{"unknown": valid}); err == nil {
		t.Fatal("unknown channel accepted")
	}
	audio := config.ChannelConfig{Doc: "audio", Seg: "grid@v1", Weight: 1, Active: []string{"clap-htsat-unfused@r1"}}
	if channels, err := NewChannels(config.ChannelsConfig{"audio": audio}); err != nil || channels.List()[0].Doc != "audio" {
		t.Fatalf("audio channel rejected: %v", err)
	}
}
