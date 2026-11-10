package archive_ops

import (
	"fmt"
	"regexp"
	"syscall"
)

// CommandProcessIdentity 是内核身份，不含命令行、环境或凭据。
type CommandProcessIdentity struct {
	PID       int    `json:"pid"`
	PGID      int    `json:"pgid"`
	UID       uint32 `json:"uid"`
	Started   string `json:"started"`
	Boot      string `json:"boot"`
	Namespace string `json:"namespace"`
}

var commandStartPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}(?::[0-9]{1,6})?$`)

func validCommandIdentity(v *CommandProcessIdentity) bool {
	return v != nil && v.PID > 0 && v.PID <= 2147483647 && v.PGID == v.PID && commandStartPattern.MatchString(v.Started) && shaPattern.MatchString(v.Boot) && shaPattern.MatchString(v.Namespace)
}
func captureCommandIdentity(pid int) (*CommandProcessIdentity, error) {
	boot, ns, e := commandBootIdentity()
	if e != nil {
		return nil, e
	}
	v, found, e := commandProcessIdentity(pid)
	if e != nil || !found {
		return nil, fmt.Errorf("command identity unavailable")
	}
	after, afterNS, e := commandBootIdentity()
	if e != nil || boot != after || ns != afterNS {
		return nil, fmt.Errorf("command boot changed")
	}
	v.Boot = boot
	v.Namespace = ns
	if !validCommandIdentity(&v) {
		return nil, fmt.Errorf("command identity invalid")
	}
	return &v, nil
}

type commandOSObservation struct {
	BootMatches bool   `json:"boot_matches"`
	Leader      string `json:"leader"`
	Group       string `json:"group"`
}

func observeRecordedCommand(r commandRecord) commandOSObservation {
	out := commandOSObservation{Leader: "unknown", Group: "unknown"}
	if r.Protocol != 2 && r.Protocol != 3 || r.State != "running" || !validCommandIdentity(r.Identity) || r.PID != r.Identity.PID || r.PGID != r.Identity.PGID {
		out.Leader = "identity_missing"
		return out
	}
	boot, ns, e := commandBootIdentity()
	if e != nil {
		return out
	}
	if boot != r.Identity.Boot || ns != r.Identity.Namespace {
		out.Leader = "boot_changed"
		return out
	}
	out.BootMatches = true
	current, found, e := commandProcessIdentity(r.PID)
	if e != nil {
		return out
	}
	if !found {
		out.Leader = "absent"
	} else if current.PID == r.Identity.PID && current.PGID == r.Identity.PGID && current.UID == r.Identity.UID && current.Started == r.Identity.Started {
		out.Leader = "matching"
	} else {
		out.Leader = "identity_mismatch"
	}
	e = syscall.Kill(-r.PGID, 0)
	switch e {
	case nil:
		out.Group = "present"
	case syscall.ESRCH:
		out.Group = "absent"
	}
	return out
}
