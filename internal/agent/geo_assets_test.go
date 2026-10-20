package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func geoFixture(tag string, count int) []byte {
	entry := protowire.AppendTag(nil, 1, protowire.BytesType)
	entry = protowire.AppendString(entry, tag)
	for range count {
		entry = protowire.AppendTag(entry, 2, protowire.BytesType)
		entry = protowire.AppendBytes(entry, []byte{1, 2})
	}
	return protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), entry)
}

func TestGeoAssetSearchCacheAndReplacement(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XRAY_LOCATION_ASSET", root)
	path := filepath.Join(root, "geosite.dat")
	raw := append(geoFixture("IR", 2), geoFixture("PRIVATE", 1)...)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var g geoCatalog
	out, err := g.browse(context.Background(), "geosite", "IR", 1)
	if err != nil || out.Matched != 1 || out.Total != 2 || out.Entries[0].Reference != "geosite:ir" || out.Entries[0].Count != 2 || len(out.SHA256) != 64 {
		t.Fatal(out, err)
	}
	oldHash := out.SHA256
	out.Entries[0].Tag = "mutated"
	out, err = g.browse(context.Background(), "geosite", "", 1)
	if err != nil || out.Matched != 2 || len(out.Entries) != 1 || out.Entries[0].Tag != "ir" {
		t.Fatal(out, err)
	}
	if err = os.WriteFile(path, geoFixture("UPDATED", 3), 0600); err != nil {
		t.Fatal(err)
	}
	out, err = g.browse(context.Background(), "geosite", "updated", 100)
	if err != nil || out.Total != 1 || out.SHA256 == oldHash {
		t.Fatal(out, err)
	}
	for _, kind := range []string{"../secrets", "/etc/passwd", "geoip.dat", ""} {
		if _, err = g.browse(context.Background(), kind, "", 100); err == nil {
			t.Fatal("accepted path", kind)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = g.browse(ctx, "geosite", "", 100); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestGeoAssetMalformedAndInstalledIndexes(t *testing.T) {
	for _, raw := range [][]byte{{}, {0xff}, {0x0a, 0xff}, geoFixture("../../secret", 1), append(geoFixture("CN", 1), geoFixture("cn", 2)...)} {
		if _, err := indexGeoAsset(context.Background(), "geoip", raw); err == nil {
			t.Fatal("accepted invalid asset")
		}
	}
	if base := os.Getenv("VELORAY_TEST_GEO_ASSETS"); base != "" {
		t.Setenv("XRAY_LOCATION_ASSET", base)
		var catalog geoCatalog
		for _, kind := range []string{"geoip", "geosite"} {
			out, err := catalog.browse(context.Background(), kind, "", 100)
			if err != nil || out.Total < 10 || len(out.Entries) != 100 {
				t.Fatal(kind, out.Total, err)
			}
		}
	}
}
