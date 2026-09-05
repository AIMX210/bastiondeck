package inventory

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// SSHConfigCandidate is a parsed Host block proposed for import (never
// written directly; the UI shows a preview and the user confirms).
type SSHConfigCandidate struct {
	Host     string `json:"host"`
	HostName string `json:"hostname"`
	User     string `json:"user"`
	Port     int    `json:"port"`
	Identity string `json:"identityFile,omitempty"`
}

// ParseSSHConfig parses a subset of OpenSSH client config: Host blocks with
// HostName/User/Port/IdentityFile keys. Wildcards and negated patterns are
// skipped (they do not describe a concrete host). Globbing-only entries such
// as "*" are dropped. Comments (#) and blank lines ignored.
func ParseSSHConfig(r io.Reader) ([]SSHConfigCandidate, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var out []SSHConfigCandidate
	var cur *SSHConfigCandidate
	flush := func() {
		if cur == nil {
			return
		}
		if cur.HostName == "" {
			cur.HostName = cur.Host
		}
		if cur.Port == 0 {
			cur.Port = 22
		}
		out = append(out, *cur)
		cur = nil
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.ToLower(fields[0])
		val := strings.Join(fields[1:], " ")
		val = strings.Trim(val, `"'`)
		switch key {
		case "host":
			flush()
			// Skip patterns: wildcards * ?, negated !, or multiple patterns.
			if strings.ContainsAny(val, "*?!") || strings.ContainsAny(val, " \t,") {
				continue
			}
			cur = &SSHConfigCandidate{Host: val}
		case "hostname":
			if cur != nil {
				cur.HostName = val
			}
		case "user":
			if cur != nil {
				cur.User = val
			}
		case "port":
			if cur != nil {
				if p, err := strconv.Atoi(val); err == nil {
					cur.Port = p
				}
			}
		case "identityfile":
			if cur != nil {
				cur.Identity = expandTilde(val)
			}
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return dedupeCandidates(out), nil
}

func expandTilde(p string) string {
	if strings.HasPrefix(p, "~/") {
		return strings.Replace(p, "~", "$HOME", 1)
	}
	return p
}

func dedupeCandidates(in []SSHConfigCandidate) []SSHConfigCandidate {
	seen := map[string]bool{}
	out := make([]SSHConfigCandidate, 0, len(in))
	for _, c := range in {
		k := c.Host + "|" + c.HostName
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, c)
	}
	return out
}
