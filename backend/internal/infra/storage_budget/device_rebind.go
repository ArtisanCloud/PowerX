package storagebudget

import (
	"encoding/json"
	"os"
	"time"
)

type deviceRebinding struct {
	PreviousDevice uint64    `json:"previous_device"`
	Device         uint64    `json:"device"`
	InstanceID     string    `json:"instance_id"`
	At             time.Time `json:"at"`
	Reason         string    `json:"reason"`
}

// 在持有主机锁时验证。即使租约已经过期，也不借换绑之机丢弃未知预留。
// 已启用清理或未知结果必须人工复核，不能在换绑后继续执行旧授权。
func (c *Coordinator) rebindPoliciesSafe() bool {
	if _, e := c.readPrivate("ownership.json", 1<<20); e == nil {
		return false
	} else if !os.IsNotExist(e) {
		return false
	}
	for _, name := range []string{"lifecycle.json", "database-retention.json", "system-table-retention.json"} {
		raw, e := c.readPrivate(name, 1<<20)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return false
		}
		var d struct {
			Version  int                        `json:"version"`
			Policy   json.RawMessage            `json:"policy"`
			Policies map[string]json.RawMessage `json:"policies"`
			Runs     []struct {
				State             string `json:"state"`
				PendingQuarantine string `json:"pending_quarantine"`
				PendingName       string `json:"pending_name"`
			} `json:"runs"`
		}
		if json.Unmarshal(raw, &d) != nil || len(d.Policies) > 64 || len(d.Runs) > 200 {
			return false
		}
		policies := []json.RawMessage{}
		if name == "database-retention.json" {
			if len(d.Policy) == 0 {
				return false
			}
			policies = append(policies, d.Policy)
		} else {
			if d.Policies == nil {
				return false
			}
			if name == "lifecycle.json" && d.Version != 1 {
				return false
			}
			for _, p := range d.Policies {
				policies = append(policies, p)
			}
		}
		for _, raw := range policies {
			var p struct {
				Enabled *bool `json:"enabled"`
			}
			if json.Unmarshal(raw, &p) != nil || p.Enabled == nil || *p.Enabled {
				return false
			}
		}
		for _, r := range d.Runs {
			if r.PendingQuarantine != "" || r.PendingName != "" {
				return false
			}
			switch r.State {
			case "completed", "failed", "blocked", "skipped", "preview":
			default:
				return false
			}
		}
	}
	return true
}
