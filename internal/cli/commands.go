package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"bastiondeck/internal/apiclient"
	"bastiondeck/internal/tui"
)

func init() {
	register(command{"tui", "tui", "launch the terminal UI", cmdTUI})
	register(command{"login", "login --url URL", "authenticate and store a token", cmdLogin})
	register(command{"logout", "logout", "remove stored credentials", cmdLogout})
	register(command{"status", "status", "show instance status", cmdStatus})
	register(command{"doctor", "doctor", "run server self-checks", cmdDoctor})
	register(command{"hosts", "hosts [ls|add|rm|test] ...", "manage hosts", cmdHosts})
	register(command{"host", "host <id>", "show one host", cmdHostShow})
	register(command{"creds", "creds [ls|add|rm] ...", "manage credentials", cmdCreds})
	register(command{"run", "run --hosts id1,id2 -- <command>", "execute across hosts", cmdRun})
	register(command{"runs", "runs [ls|show|cancel] ...", "inspect runs", cmdRuns})
	register(command{"runx", "runx <runId>", "watch a run to completion with output", cmdRunWatch})
	register(command{"snippets", "snippets [ls]", "list snippets", cmdSnippets})
	register(command{"audit", "audit [ls|verify]", "audit trail", cmdAudit})
	register(command{"tunnels", "tunnels [ls]", "list tunnels", cmdTunnels})
	register(command{"agents", "agents [ls|enroll|approve|block]", "manage agents", cmdAgents})
	register(command{"backup", "backup export --passphrase X", "encrypted backup", cmdBackup})
}

func prompt(p string) string {
	fmt.Fprint(os.Stdout, p)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

func cmdTUI(ctx context.Context, a *App, args []string) error {
	c, _, err := a.client(args)
	if err != nil {
		return err
	}
	return tui.Run(c)
}

func cmdLogin(ctx context.Context, a *App, args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	urlF := fs.String("url", "http://127.0.0.1:8840", "API base URL")
	userF := fs.String("user", "", "username (prompted if empty)")
	passF := fs.String("password", "", "password (prompted if empty)")
	totpF := fs.String("totp", "", "TOTP code")
	if err := fs.Parse(args); err != nil {
		return err
	}
	username := *userF
	if username == "" {
		username = prompt("username: ")
	}
	password := *passF
	if password == "" {
		password = prompt("password: ")
	}
	c := apiclient.New(*urlF, "")
	token, u, err := c.Login(ctx, username, password, *totpF)
	if err != nil {
		return err
	}
	cfg := &Config{BaseURL: *urlF, Token: token, User: u.Username, Role: u.Role}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "logged in as %s (%s), profile saved to %s\n", u.Username, u.Role, ConfigPath())
	return nil
}

func cmdLogout(ctx context.Context, a *App, args []string) error {
	if err := Clear(); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "logged out")
	return nil
}

func cmdStatus(ctx context.Context, a *App, args []string) error {
	c, rest, err := a.client(args)
	if err != nil {
		return err
	}
	_ = rest
	st, err := c.Status(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "BastionDeck %s  setupRequired=%v\n", st.Version, st.SetupRequired)
	return nil
}

func cmdDoctor(ctx context.Context, a *App, args []string) error {
	c, _, err := a.client(args)
	if err != nil {
		return err
	}
	rep, err := c.Doctor(ctx)
	if err != nil {
		return err
	}
	ok, _ := rep["ok"].(bool)
	fmt.Fprintf(a.Out, "doctor ok=%v\n", ok)
	if checks, ok := rep["checks"].([]any); ok {
		for _, x := range checks {
			m := x.(map[string]any)
			fmt.Fprintf(a.Out, "  %-20s %v %v\n", m["name"], m["ok"], orDash(m["detail"]))
		}
	}
	return nil
}

func orDash(v any) string {
	if v == nil {
		return ""
	}
	s := fmt.Sprint(v)
	if s == "" {
		return ""
	}
	return s
}

func cmdHosts(ctx context.Context, a *App, args []string) error {
	c, rest, err := a.client(args)
	if err != nil {
		return err
	}
	action := "ls"
	if len(rest) > 0 {
		action, rest = rest[0], rest[1:]
	}
	switch action {
	case "ls":
		hosts, err := c.ListHosts(ctx, strings.Join(rest, " "))
		if err != nil {
			return err
		}
		t := newTable(a.Out, "ID", "NAME", "ADDRESS", "USER", "STATUS", "TAGS")
		t.head()
		for _, h := range hosts {
			t.row(shortID(h.ID), h.Name, fmt.Sprintf("%s:%d", h.Address, h.Port),
				h.Username, h.Status, strings.Join(h.Tags, ","))
		}
		t.flush()
		fmt.Fprintf(a.Out, "%d host(s)\n", len(hosts))
	case "add":
		fs := flag.NewFlagSet("hosts add", flag.ContinueOnError)
		name := fs.String("name", "", "host name")
		addr := fs.String("addr", "", "address")
		port := fs.Int("port", 22, "port")
		user := fs.String("user", "root", "username")
		cred := fs.String("cred", "", "credential id")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		h, err := c.CreateHost(ctx, map[string]any{
			"name": *name, "address": *addr, "port": *port, "username": *user,
			"credentialId": *cred, "authKind": "credential"})
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "created %s\n", h.ID)
	case "rm":
		if len(rest) == 0 {
			return fmt.Errorf("hosts rm <id>")
		}
		for _, id := range rest {
			if err := c.DeleteHost(ctx, id); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "removed %s\n", id)
		}
	case "test":
		if len(rest) == 0 {
			return fmt.Errorf("hosts test <id>")
		}
		res, err := c.TestHost(ctx, rest[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "%v\n", res)
	default:
		return fmt.Errorf("unknown hosts action %q", action)
	}
	return nil
}

func cmdHostShow(ctx context.Context, a *App, args []string) error {
	c, rest, err := a.client(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("host <id>")
	}
	hosts, err := c.ListHosts(ctx, "")
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if h.ID == rest[0] || shortID(h.ID) == rest[0] {
			fmt.Fprintf(a.Out, "%#v\n", h)
			return nil
		}
	}
	return fmt.Errorf("host not found: %s", rest[0])
}

func cmdCreds(ctx context.Context, a *App, args []string) error {
	c, rest, err := a.client(args)
	if err != nil {
		return err
	}
	action := "ls"
	if len(rest) > 0 {
		action, rest = rest[0], rest[1:]
	}
	switch action {
	case "ls":
		cs, err := c.ListCredentials(ctx)
		if err != nil {
			return err
		}
		t := newTable(a.Out, "ID", "NAME", "KIND", "FINGERPRINT")
		t.head()
		for _, x := range cs {
			t.row(shortID(x.ID), x.Name, x.Kind, truncate(x.Fingerprint, 24))
		}
		t.flush()
	case "add":
		fs := flag.NewFlagSet("creds add", flag.ContinueOnError)
		name := fs.String("name", "", "name")
		kind := fs.String("kind", "password", "password|private_key")
		secret := fs.String("secret", "", "secret value")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		cd, err := c.CreateCredential(ctx, *name, *kind, *secret)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "created %s\n", cd.ID)
	case "rm":
		if len(rest) == 0 {
			return fmt.Errorf("creds rm <id>")
		}
		_ = rest
		return fmt.Errorf("delete credentials via API (DELETE /api/credentials/:id)")
	default:
		return fmt.Errorf("unknown creds action %q", action)
	}
	return nil
}

func cmdRun(ctx context.Context, a *App, args []string) error {
	c, rest, err := a.client(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	hostsF := fs.String("hosts", "", "comma separated host ids")
	groupF := fs.String("group", "", "group id")
	timeoutF := fs.Int("timeout", 60, "per-host timeout seconds")
	watch := fs.Bool("watch", true, "watch until completion")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	cmdArgs := fs.Args()
	if len(cmdArgs) == 0 {
		return fmt.Errorf("provide a command after --")
	}
	command := strings.Join(cmdArgs, " ")
	var ids []string
	if *hostsF != "" {
		ids = strings.Split(*hostsF, ",")
	}
	// exec endpoint accepts groupId; apiclient.Exec currently takes ids only,
	// so resolve a group to ids via listing when provided.
	if *groupF != "" {
		hs, err := c.ListHosts(ctx, "")
		if err != nil {
			return err
		}
		for _, h := range hs {
			if h.GroupID != nil && *h.GroupID == *groupF {
				ids = append(ids, h.ID)
			}
		}
	}
	runID, err := c.Exec(ctx, command, ids, *timeoutF)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "run %s started\n", runID)
	if *watch {
		return watchRun(ctx, a, c, runID)
	}
	return nil
}

func watchRun(ctx context.Context, a *App, c *apiclient.Client, runID string) error {
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		run, _, err := c.GetRun(ctx, runID)
		if err != nil {
			return err
		}
		if isTerminal(run.Status) {
			t := newTable(a.Out, "HOST", "STATUS", "EXIT", "NOTE")
			t.head()
			for _, tg := range run.Targets {
				exit := "-"
				if tg.ExitCode != nil {
					exit = fmt.Sprint(*tg.ExitCode)
				}
				t.row(shortID(tg.HostID), statusGlyph(tg.Status)+" "+tg.Status, exit, truncate(tg.ErrorText, 40))
			}
			t.flush()
			fmt.Fprintf(a.Out, "run %s => %s (ok=%d failed=%d lost=%d)\n", runID, run.Status,
				run.Summary.Success, run.Summary.Failed, run.Summary.Lost)
			if run.Status != "success" {
				return fmt.Errorf("run %s", run.Status)
			}
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("watch timed out")
}

func isTerminal(s string) bool {
	switch s {
	case "success", "failed", "timeout", "cancelled", "lost":
		return true
	}
	return false
}

func cmdRuns(ctx context.Context, a *App, args []string) error {
	c, rest, err := a.client(args)
	if err != nil {
		return err
	}
	action := "ls"
	if len(rest) > 0 {
		action, rest = rest[0], rest[1:]
	}
	switch action {
	case "ls":
		runs, err := c.ListRuns(ctx, 20)
		if err != nil {
			return err
		}
		t := newTable(a.Out, "RUN", "JOB", "STATUS", "OK", "FAIL", "LOST")
		t.head()
		for _, r := range runs {
			t.row(shortID(r.ID), shortID(r.JobID), r.Status,
				fmt.Sprint(r.Summary.Success), fmt.Sprint(r.Summary.Failed), fmt.Sprint(r.Summary.Lost))
		}
		t.flush()
	case "show":
		if len(rest) == 0 {
			return fmt.Errorf("runs show <id>")
		}
		run, _, err := c.GetRun(ctx, rest[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "run %s status=%s\n", run.ID, run.Status)
		for _, tg := range run.Targets {
			fmt.Fprintf(a.Out, "  %s %s\n", shortID(tg.HostID), tg.Status)
			if tg.StdoutPreview != "" {
				fmt.Fprintf(a.Out, "    out: %s\n", truncate(tg.StdoutPreview, 120))
			}
			if tg.StderrPreview != "" {
				fmt.Fprintf(a.Out, "    err: %s\n", truncate(tg.StderrPreview, 120))
			}
		}
	case "cancel":
		if len(rest) == 0 {
			return fmt.Errorf("runs cancel <id>")
		}
		if err := c.CancelRun(ctx, rest[0]); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "cancelled")
	default:
		return fmt.Errorf("unknown runs action %q", action)
	}
	return nil
}

func cmdRunWatch(ctx context.Context, a *App, args []string) error {
	c, rest, err := a.client(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("runx <runId>")
	}
	return watchRun(ctx, a, c, rest[0])
}

func cmdSnippets(ctx context.Context, a *App, args []string) error {
	c, _, err := a.client(args)
	if err != nil {
		return err
	}
	sn, err := c.ListSnippets(ctx)
	if err != nil {
		return err
	}
	t := newTable(a.Out, "ID", "TITLE", "TAGS")
	t.head()
	for _, x := range sn {
		t.row(shortID(x.ID), x.Title, strings.Join(x.Tags, ","))
	}
	t.flush()
	return nil
}

func cmdAudit(ctx context.Context, a *App, args []string) error {
	c, rest, err := a.client(args)
	if err != nil {
		return err
	}
	action := "ls"
	if len(rest) > 0 {
		action = rest[0]
	}
	if action == "verify" {
		rep, err := c.VerifyAudit(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "%v\n", rep)
		return nil
	}
	entries, err := c.ListAudit(ctx, 30)
	if err != nil {
		return err
	}
	t := newTable(a.Out, "TIME", "ACTOR", "ACTION", "OBJECT", "RESULT")
	t.head()
	for _, e := range entries {
		t.row(e.At, e.ActorName, e.Action, shortID(e.ObjectID), e.Result)
	}
	t.flush()
	return nil
}

func cmdTunnels(ctx context.Context, a *App, args []string) error {
	c, _, err := a.client(args)
	if err != nil {
		return err
	}
	_ = c
	fmt.Fprintln(a.Out, "tunnels are managed via the web UI or API (POST /api/tunnels)")
	return nil
}

func cmdAgents(ctx context.Context, a *App, args []string) error {
	c, _, err := a.client(args)
	if err != nil {
		return err
	}
	_ = c
	fmt.Fprintln(a.Out, "agents are managed via the web UI or API")
	return nil
}

func cmdBackup(ctx context.Context, a *App, args []string) error {
	c, rest, err := a.client(args)
	if err != nil {
		return err
	}
	_ = rest
	_ = c
	fmt.Fprintln(a.Out, "use POST /api/backup/export (owner only) via the web UI")
	return nil
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
