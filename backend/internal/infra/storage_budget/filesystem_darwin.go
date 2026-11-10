//go:build darwin

package storagebudget

import (
	"bytes"
	"context"
	"encoding/xml"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Darwin 的 st_dev 会在重新挂载后变化；只有配置中的卷 UUID 才能核验原卷。
func stableFilesystemMatches(path, id string) bool {
	if !strings.HasPrefix(id, "fs-") {
		return false
	}
	expected, e := uuid.Parse(strings.TrimPrefix(id, "fs-"))
	if e != nil {
		return false
	}
	var st unix.Statfs_t
	if unix.Statfs(path, &st) != nil {
		return false
	}
	mount := strings.TrimRight(string(st.Mntonname[:]), "\x00")
	if !strings.HasPrefix(mount, "/") {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, e := exec.CommandContext(ctx, "/usr/sbin/diskutil", "info", "-plist", mount).Output()
	if e != nil || len(raw) > 128<<10 {
		return false
	}
	actual, ok := parseVolumeUUID(raw)
	return ok && actual == expected
}

func parseVolumeUUID(raw []byte) (uuid.UUID, bool) {
	if len(raw) > 128<<10 {
		return uuid.Nil, false
	}
	d := xml.NewDecoder(bytes.NewReader(raw))
	depth := 0
	pending := false
	found := false
	value := uuid.Nil
	for {
		token, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return uuid.Nil, false
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 3 && t.Name.Local == "key" {
				var key string
				if d.DecodeElement(&key, &t) != nil {
					return uuid.Nil, false
				}
				depth--
				pending = key == "VolumeUUID"
			} else if depth == 3 && pending {
				pending = false
				if t.Name.Local != "string" || found {
					return uuid.Nil, false
				}
				var text string
				if d.DecodeElement(&text, &t) != nil {
					return uuid.Nil, false
				}
				depth--
				parsed, e := uuid.Parse(text)
				if e != nil || parsed == uuid.Nil {
					return uuid.Nil, false
				}
				value = parsed
				found = true
			}
		case xml.EndElement:
			depth--
		}
	}
	return value, found
}
